package watch

import (
	"os"
	"path"
	"time"

	"github.com/rjeczalik/notify"
)

// An existenceChecker checks the existence of a file
type existenceChecker interface {
	Check(p string) bool
}

type statChecker struct{}

func (sc statChecker) Check(p string) bool {
	_, err := os.Stat(p)
	if err == nil {
		return true
	}
	return false
}

// This function batches events up, and emits just a list of paths for files
// considered changed. It applies some heuristics to deal with short-lived
// temporary files.
//
// - Events can arrive out of order - i.e. we can get a removal notice first
// then a creation notice for a transient file.
func batch(batchTime time.Duration, exists existenceChecker, ch chan notify.EventInfo) []string {
	emap := make(map[string]bool)
	for {
		select {
		case evt := <-ch:
			emap[evt.Path()] = true
		case <-time.After(batchTime):
			var ret []string
			for k := range emap {
				if exists.Check(k) {
					ret = append(ret, k)
				}
			}
			return ret
		}
	}
}

// Watch watches a path p, batching events with duration batchTime. A list of
// strings are written to chan, representing all files changed, added or
// removed. We apply heuristics to do things like cope with transient
// files.
func Watch(p string, batchTime time.Duration, ch chan []string) error {
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
			for {
				ret := batch(batchTime, statChecker{}, evtch)
				if len(ret) > 0 {
					ch <- ret
				}
			}
		}()
	}
	return err
}
