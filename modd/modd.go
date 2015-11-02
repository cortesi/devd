package modd

import (
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/rjeczalik/notify"
)

// Notify events have absolute paths. We want to normalize these so that they
// are relative to the base path.
func normPath(base string, abspath string) (string, error) {
	absbase, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}
	relpath, err := filepath.Rel(absbase, abspath)
	if err != nil {
		return "", err
	}
	return filepath.Join(base, relpath), nil
}

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

// Mod encapsulates a set of changes
type Mod struct {
	Changed []string
	Deleted []string
	Added   []string
}

func (*Mod) normPath(base string) error {
	// for i := range ret {
	// 	norm, _ := normPath(p, ret[i])
	// 	ret[i] = norm
	// }

	return nil
}

func newMod() *Mod {
	return &Mod{
		make([]string, 0),
		make([]string, 0),
		make([]string, 0),
	}
}

// This function batches events up, and emits just a list of paths for files
// considered changed. It applies some heuristics to deal with short-lived
// temporary files.
//
// - Events can arrive out of order - i.e. we can get a removal event first
// then a creation event for a transient file.
// - Events seem to be unreliable on some platforms - i.e. we might get a
// removal event but never see a creation event.
func batch(batchTime time.Duration, exists existenceChecker, ch chan notify.EventInfo) *Mod {
	created := make(map[string]bool)
	removed := make(map[string]bool)
	changed := make(map[string]bool)
	for {
		select {
		case evt := <-ch:
			switch evt.Event() {
			case notify.Create:
				created[evt.Path()] = true
			case notify.Remove:
				removed[evt.Path()] = true
			case notify.Write:
				changed[evt.Path()] = true
			case notify.Rename:
				created[evt.Path()] = true
			}
		case <-time.After(batchTime):
			ret := newMod()
			for k := range changed {
				if exists.Check(k) {
					//ret = append(ret, k)
				}
			}
			return ret
		}
	}
}

// Watch watches a path p, batching events with duration batchTime. A list of
// strings are written to chan, representing all files changed, added or
// removed. We apply heuristics to cope with things like transient files and
// unreliable event notifications.
func Watch(p string, batchTime time.Duration, ch chan Mod) error {
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
				if ret != nil {
					ret.normPath(p)
					ch <- *ret
				}
			}
		}()
	}
	return err
}
