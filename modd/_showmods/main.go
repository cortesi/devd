package main
// A simple demo app that just prints all files changed for a path

import (
	"fmt"
	"time"

	"github.com/cortesi/devd/modd"
	"github.com/cortesi/kingpin"
)

const batchTime = time.Millisecond * 200

func main() {
	paths := kingpin.Arg(
		"path",
		"Paths to monitor for changes.",
	).Required().Strings()
	kingpin.Parse()

	pathchan := make(chan []string)
	for _, path := range *paths {
		err := modd.Watch(path, batchTime, pathchan)
		if err != nil {
			kingpin.Fatalf("Fatal error: %s", err)
		}
	}
	for paths := range pathchan {
		fmt.Println(paths)
	}
}
