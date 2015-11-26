package devd

import (
	"time"

	"github.com/bmatcuk/doublestar"
	"github.com/cortesi/devd/livereload"
	"github.com/cortesi/modd"
	"github.com/cortesi/termlog"
)

const batchTime = time.Millisecond * 200

// Watch watches an endpoint for changes, if it supports them.
func (r Route) Watch(ch chan []string, excludePatterns []string, log termlog.Logger) error {
	switch r.Endpoint.(type) {
	case *filesystemEndpoint:
		ep := *r.Endpoint.(*filesystemEndpoint)
		modchan := make(chan modd.Mod, 1)
		err := modd.Watch([]string{string(ep)}, batchTime, modchan)
		if err != nil {
			return err
		}
		go func() {
			for mod := range modchan {
				files := filterFiles(mod.All(), excludePatterns, log)
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
func filterFiles(files, excludePatterns []string, log termlog.Logger) []string {
	ret := []string{}
	for _, file := range files {
		if shouldInclude(file, excludePatterns, log) {
			ret = append(ret, file)
		}
	}
	return ret
}

// WatchPaths watches a set of paths, and broadcasts changes through reloader.
func WatchPaths(paths, excludePatterns []string, reloader livereload.Reloader, log termlog.Logger) error {
	ch := make(chan []string, 1)
	for _, path := range paths {
		modchan := make(chan modd.Mod, 1)
		err := modd.Watch([]string{path}, batchTime, modchan)
		if err != nil {
			return err
		}
		go func() {
			for mod := range modchan {
				files := filterFiles(mod.All(), excludePatterns, log)
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
