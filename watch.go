package devd

import (
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar"
	"github.com/cortesi/devd/livereload"
	"github.com/cortesi/devd/termlog"
	"github.com/rjeczalik/notify"
)

const batchTime = time.Millisecond * 200

// This function batches events up, and emits just a list of paths for files
// considered changed. It applies some heuristics to deal with short-lived
// temporary files.
func batch(batchTime time.Duration, ch chan string) chan []string {
	ret := make(chan []string)
	go func() {
		emap := make(map[string]bool)
		for {
			select {
			case path := <-ch:
				emap[path] = true
			case <-time.After(batchTime):
				keys := make([]string, 0, len(emap))
				for k := range emap {
					_, err := os.Stat(k)
					if err == nil {
						keys = append(keys, k)
					}
				}
				if len(keys) > 0 {
					ret <- keys
				}
				emap = make(map[string]bool)
			}
		}
	}()
	return ret
}

func watch(p string, ch chan string) error {
	stat, err := os.Stat(p)
	if err != nil {
		return err
	}
	if stat.IsDir() {
		p = path.Join(p, "...")
	}
	evtch := make(chan notify.EventInfo)
	err = notify.Watch(p, evtch, notify.All)
	if err == nil {
		go func() {
			for e := range evtch {
				ch <- e.Path()
			}
		}()
	}
	return err
}

// Watch watches an endpoint for changes, if it supports them.
func (r Route) Watch(ch chan []string, excludePatterns []string, log termlog.Logger) error {
	switch r.Endpoint.(type) {
	case *filesystemEndpoint:
		ep := *r.Endpoint.(*filesystemEndpoint)
		pathchan := make(chan string, 1)
		err := watch(string(ep), pathchan)
		if err != nil {
			return err
		}
		go func() {
			for files := range batch(batchTime, pathchan) {
				for i, fpath := range files {
					files[i] = path.Join(
						r.Path,
						strings.TrimPrefix(fpath, string(ep)),
					)
				}
				files = filterFiles("/", files, excludePatterns, log)
				ch <- files
			}
		}()
	}
	return nil
}

// Determine if a file should be included, based on the given exclude paths.
func shouldInclude(file string, excludePatterns []string, log termlog.Logger) bool {
	for _, pattern := range excludePatterns {
		match, err := doublestar.Match(pattern, file)
		if err != nil {
			log.Warn("Error matching pattern '%s': %s", pattern, err)
		} else if match {
			return false
		}
	}
	return true
}

// Filter out the files that match the given exclude patterns.
func filterFiles(pathPrefix string, files, excludePatterns []string, log termlog.Logger) []string {
	ret := []string{}
	for _, file := range files {
		relFile := strings.TrimPrefix(file, pathPrefix)
		if shouldInclude(relFile, excludePatterns, log) {
			ret = append(ret, file)
		}
	}
	return ret
}

// WatchPaths watches a set of paths, and broadcasts changes through reloader.
func WatchPaths(paths, excludePatterns []string, reloader livereload.Reloader, log termlog.Logger) error {
	ch := make(chan []string, 1)
	for _, path := range paths {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		if absPath[len(absPath)-1] != filepath.Separator {
			absPath += string(filepath.Separator)
		}

		pathchan := make(chan string, 1)

		err = watch(path, pathchan)
		if err != nil {
			return err
		}

		go func() {
			for files := range batch(batchTime, pathchan) {
				files = filterFiles(absPath, files, excludePatterns, log)

				if len(files) > 0 {
					ch <- files
				}
			}
		}()
	}
	go reloader.Watch(ch)
	return nil
}

// WatchRoutes watches the route collection, and broadcasts changes through reloader.
func WatchRoutes(routes RouteCollection, reloader livereload.Reloader, excludePatterns []string, log termlog.Logger) error {
	c := make(chan []string, 1)
	for i := range routes {
		err := routes[i].Watch(c, excludePatterns, log)
		if err != nil {
			return err
		}
	}
	go reloader.Watch(c)
	return nil
}
