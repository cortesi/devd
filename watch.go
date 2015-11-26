package devd

import (
	"time"

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
		err := modd.Watch([]string{string(ep)}, excludePatterns, batchTime, modchan)
		if err != nil {
			return err
		}
		go func() {
			for mod := range modchan {
				ch <- mod.All()
			}
		}()
	}
	return nil
}

// WatchPaths watches a set of paths, and broadcasts changes through reloader.
func WatchPaths(paths, excludePatterns []string, reloader livereload.Reloader, log termlog.Logger) error {
	ch := make(chan []string, 1)
	for _, path := range paths {
		modchan := make(chan modd.Mod, 1)
		err := modd.Watch([]string{path}, excludePatterns, batchTime, modchan)
		if err != nil {
			return err
		}
		go func() {
			for mod := range modchan {
				ch <- mod.All()
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
