package modd

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
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

func normPaths(base string, abspaths []string) error {
	for i, p := range abspaths {
		norm, err := normPath(base, p)
		if err != nil {
			return err
		}
		abspaths[i] = norm
	}
	return nil
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

// All returns a single list of all changed files
func (mod Mod) All() []string {
	all := make(map[string]bool)
	for _, p := range mod.Changed {
		all[p] = true
	}
	for _, p := range mod.Added {
		all[p] = true
	}
	for _, p := range mod.Deleted {
		all[p] = true
	}
	return _keys(all)
}

// Empty checks if this mod set is empty
func (mod Mod) Empty() bool {
	if len(mod.Changed) > 0 || len(mod.Deleted) > 0 || len(mod.Added) > 0 {
		return false
	}
	return true
}

func (mod *Mod) normPaths(base string) error {
	if err := normPaths(base, mod.Changed); err != nil {
		return err
	}
	if err := normPaths(base, mod.Deleted); err != nil {
		return err
	}
	if err := normPaths(base, mod.Added); err != nil {
		return err
	}
	return nil
}

func _keys(m map[string]bool) []string {
	if len(m) > 0 {
		keys := make([]string, len(m))
		i := 0
		for k := range m {
			keys[i] = k
			i++
		}
		sort.Strings(keys)
		return keys
	}
	return nil
}

// This function batches events up, and emits just a list of paths for files
// considered changed. It applies some heuristics to deal with short-lived
// temporary files and unreliable filesystem events.
//
// - Events can arrive out of order - i.e. we can get a removal event first
// then a creation event for a transient file.
// - Events seem to be unreliable on some platforms - i.e. we might get a
// removal event but never see a creation event.
// - Events are nonsensical on some platforms - i.e. we sometimes get a Create
// event as well as a Remove event when a pre-existing file is removed.
//
// In the face of all this, all we can do is layer on a set of heuristics to
// try to get intuitive results.
func batch(base string, batchTime time.Duration, exists existenceChecker, ch chan notify.EventInfo) *Mod {
	added := make(map[string]bool)
	removed := make(map[string]bool)
	changed := make(map[string]bool)
	renamed := make(map[string]bool)
	for {
		select {
		case evt := <-ch:
			fmt.Println(evt)
			switch evt.Event() {
			case notify.Create:
				added[evt.Path()] = true
			case notify.Remove:
				removed[evt.Path()] = true
			case notify.Write:
				changed[evt.Path()] = true
			case notify.Rename:
				renamed[evt.Path()] = true
			}
		case <-time.After(batchTime):
			ret := &Mod{}
			for k := range renamed {
				// If a file has been renamed AND exists, we put it in added.
				// Editors commonly rename files to and from temporary names
				// during editing.
				if exists.Check(k) {
					added[k] = true
				} else if _, ok := removed[k]; ok {
					// If a file was renamed, doesn't exist, and has been
					// removed, we remove it everywhere.
					delete(added, k)
					delete(removed, k)
					delete(changed, k)
				}
			}
			for k := range added {
				if exists.Check(k) {
					// If a file exists, and has been both added and
					// changed, we just mark it as added
					delete(changed, k)
					delete(removed, k)
				} else {
					// If a file has been added, and now does not exist, we
					// strike it everywhere. This probably means the file is
					// transient - i.e. has been quickly added and removed, or
					// we've just not recieved a removal notification.
					delete(added, k)
					delete(removed, k)
					delete(changed, k)
				}
			}
			ret.Added = _keys(added)
			ret.Changed = _keys(changed)
			ret.Deleted = _keys(removed)
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
	evtch := make(chan notify.EventInfo, 1024)
	err = notify.Watch(p, evtch, notify.All)
	if err == nil {
		go func() {
			for {
				ret := batch(p, batchTime, statChecker{}, evtch)
				if ret != nil {
					ret.normPaths(p)
					if !ret.Empty() {
						ch <- *ret
					}
				}
			}
		}()
	}
	return err
}
