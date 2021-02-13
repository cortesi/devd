package devd

import (
	"os"
	"time"

	"github.com/milanaleksic/devd/livereload"
	"github.com/cortesi/moddwatch"
	"github.com/cortesi/termlog"
)

const batchTime = time.Millisecond * 200

// Watch watches an endpoint for changes, if it supports them.
func (r Route) Watch(
	ch chan []string,
	excludePatterns []string,
	log termlog.Logger,
) (*moddwatch.Watcher, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	var watcher *moddwatch.Watcher
	switch r.Endpoint.(type) {
	case *filesystemEndpoint:
		ep := *r.Endpoint.(*filesystemEndpoint)
		modchan := make(chan *moddwatch.Mod, 1)
		watcher, err = moddwatch.Watch(
			wd,
			[]string{ep.Root + "/...", "**"},
			excludePatterns,
			batchTime,
			modchan,
		)
		if err != nil {
			return nil, err
		}
		go func() {
			for mod := range modchan {
				if !mod.Empty() {
					ch <- mod.All()
				}
			}
		}()
	}
	return watcher, nil
}

// WatchPaths watches a set of paths, and broadcasts changes through reloader.
func WatchPaths(paths, excludePatterns []string, reloader livereload.Reloader, log termlog.Logger) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	ch := make(chan []string, 1)
	for _, path := range paths {
		modchan := make(chan *moddwatch.Mod, 1)
		_, err := moddwatch.Watch(
			wd,
			[]string{path},
			excludePatterns,
			batchTime,
			modchan,
		)
		if err != nil {
			return err
		}
		go func() {
			for mod := range modchan {
				if !mod.Empty() {
					ch <- mod.All()
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
		_, err := routes[i].Watch(c, excludePatterns, log)
		if err != nil {
			return err
		}
	}
	go reloader.Watch(c)
	return nil
}
