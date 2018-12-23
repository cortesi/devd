package devd

import (
	"io/ioutil"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cortesi/moddwatch"
	"github.com/cortesi/termlog"
)

func addTempFile(wg *sync.WaitGroup, t *testing.T, tmpFolder string, fname string, content string) {
	if err := ioutil.WriteFile(tmpFolder+"/"+fname, []byte(content), 0644); err != nil {
		t.Error(err)
	}
	wg.Add(1)
}

// waitTimeout waits for the waitgroup for the specified max timeout.
// Returns true if waiting timed out.
func waitTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()
	select {
	case <-c:
		return false // completed normally
	case <-time.After(timeout):
		return true // timed out
	}
}

func TestRouteWatch(t *testing.T) {
	logger := termlog.NewLog()
	logger.Quiet()

	tmpFolder, err := ioutil.TempDir("", "")
	if err != nil {
		t.Error(err)
	}
	defer os.RemoveAll(tmpFolder)

	// Ensure that using . for the path works:
	os.Chdir(tmpFolder)
	routes := make(RouteCollection)
	routes.Add(".", nil)

	changedFiles := make(map[string]int)
	ch := make(chan []string, 1)
	var wg sync.WaitGroup
	go func() {
		for {
			data, more := <-ch
			if more {
				t.Log("received notification for changed file(s):", strings.Join(data, ", "))
				for i := range data {
					changedFiles[data[i]] = 1
					wg.Done()
				}
			} else {
				t.Log("No more changes are expected")
				return
			}
		}
	}()
	watchers := make([]*moddwatch.Watcher, len(routes))
	i := 0
	for r := range routes {
		watcher, err := routes[r].Watch(ch, nil, logger)
		watchers[i] = watcher
		if err != nil {
			t.Error(err)
		}
		i++
	}
	addTempFile(&wg, t, tmpFolder, "a.txt", "foo\n")
	addTempFile(&wg, t, tmpFolder, "c.txt", "bar\n")
	addTempFile(&wg, t, tmpFolder, "another.file.txt", "bar\n")
	waitTimeout(&wg, 5*time.Second)

	for _, v := range watchers {
		v.Stop()
	}

	close(ch)
	if len(changedFiles) != 3 {
		t.Error("The watch should have been notified about 3 changed files")
	}
}
