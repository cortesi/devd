package devd

import (
	"fmt"
	"io/ioutil"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/cortesi/moddwatch"
	"github.com/cortesi/termlog"
)

func addTempFile(t *testing.T, tmpFolder string, fname string, content string) {
	if err := ioutil.WriteFile(tmpFolder+"/"+fname, []byte(content), 0644); err != nil {
		t.Error(err)
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
	ch := make(chan []string, 1024)

	var exited sync.WaitGroup
	exited.Add(1)
	var lck sync.Mutex
	go func() {
		for {
			data, more := <-ch
			if more {
				for i := range data {
					lck.Lock()
					fmt.Println(data)
					if _, ok := changedFiles[data[i]]; !ok {
						changedFiles[data[i]] = 1
					}
					lck.Unlock()
				}
			} else {
				exited.Done()
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

	addTempFile(t, tmpFolder, "a.txt", "foo\n")
	addTempFile(t, tmpFolder, "c.txt", "bar\n")
	addTempFile(t, tmpFolder, "another.file.txt", "bar\n")

	for i := 0; i < 100; i++ {
		lck.Lock()
		if len(changedFiles) >= 3 {
			lck.Unlock()
			break
		}
		lck.Unlock()
		time.Sleep(50 * time.Millisecond)
	}

	for _, v := range watchers {
		v.Stop()
	}
	close(ch)

	exited.Wait()

	if len(changedFiles) != 3 {
		t.Errorf("wanted 3 changed files, got %d", len(changedFiles))
	}
}
