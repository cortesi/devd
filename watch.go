package devd

import (
	"time"

	"github.com/cortesi/devd/livereload"
	"github.com/cortesi/moddwatch"
	"github.com/cortesi/termlog"
)

const batchTime = time.Millisecond * 200

// Watch watches an endpoint for changes, if it supports them.
func (r Route) Watch(ch chan []string, excludePatterns []string, log termlog.Logger) error {
	switch r.Endpoint.(type) {
	case *filesystemEndpoint:
		ep := *r.Endpoint.(*filesystemEndpoint)
		modchan := make(chan *moddwatch.Mod, 1)
		_, err := moddwatch.Watch([]string{string(ep) + "/..."}, batchTime, modchan)
		if err != nil {
			return err
		}
		go func() {
			for mod := range modchan {
				filteredMod, err := mod.Filter([]string{"**/*"}, excludePatterns)
				if err != nil {
					log.Shout("Error filtering watches: %s", err)
				}
				if !filteredMod.Empty() {
					ch <- filteredMod.All()
				}
			}
		}()
	}
	return nil
}

// WatchPaths watches a set of paths, and broadcasts changes through reloader.
func WatchPaths(paths, excludePatterns []string, reloader livereload.Reloader, log termlog.Logger) error {
	ch := make(chan []string, 1)
	for _, path := range paths {
		modchan := make(chan *moddwatch.Mod, 1)
		_, err := moddwatch.Watch([]string{path}, batchTime, modchan)
		if err != nil {
			return err
		}
		go func() {
			for mod := range modchan {
				filteredMod, err := mod.Filter([]string{"**/*"}, excludePatterns)
				if err != nil {
					log.Shout("Error filtering watches: %s", err)
				}
				if !filteredMod.Empty() {
					ch <- filteredMod.All()
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
