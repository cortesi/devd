// Package livereload allows HTML pages to be dynamically reloaded. It includes
// both the server and client implementations required.
package livereload

import (
	"net/http"
	"regexp"
	"strings"
	"sync"

	"github.com/GeertJohan/go.rice"
	"github.com/cortesi/devd/inject"
	"github.com/cortesi/termlog"
	"github.com/gorilla/websocket"
)

// Reloader triggers a reload
type Reloader interface {
	Reload(paths []string)
	Watch(ch chan []string)
}

const (
	cmdPage = "page"
	cmdCSS  = "css"
	// EndpointPath is the path to the websocket endpoint
	EndpointPath = "/.devd.livereload"
	// ScriptPath is the path to the livereload JavaScript asset
	ScriptPath = "/.devd.livereload.js"
)

// Injector for the livereload script
var Injector = inject.CopyInject{
	Within:  1024 * 30,
	Marker:  regexp.MustCompile(`<\/head>`),
	Payload: []byte(`<script src="/.devd.livereload.js"></script>`),
}

// Server implements a Livereload server
type Server struct {
	sync.Mutex
	broadcast chan<- string

	logger      termlog.Logger
	name        string
	connections map[*websocket.Conn]bool
}

// NewServer createss a Server instance
func NewServer(name string, logger termlog.Logger) *Server {
	broadcast := make(chan string, 50)
	s := &Server{
		name:        name,
		broadcast:   broadcast,
		connections: make(map[*websocket.Conn]bool),
		logger:      logger,
	}
	go s.run(broadcast)
	return s
}

func (s *Server) run(broadcast <-chan string) {
	for m := range broadcast {
		s.Lock()
		for conn := range s.connections {
			if conn == nil {
				continue
			}
			err := conn.WriteMessage(websocket.TextMessage, []byte(m))
			if err != nil {
				s.logger.Say("Error: %s", err)
				delete(s.connections, conn)
			}
		}
		s.Unlock()
	}
	s.Lock()
	defer s.Unlock()
	for conn := range s.connections {
		delete(s.connections, conn)
		conn.Close()
	}
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Say("Error: %s", err)
		http.Error(w, "Can't upgrade.", 500)
		return
	}
	s.Lock()
	s.connections[conn] = true
	s.Unlock()
}

// Reload signals to connected clients that a given resource should be
// reloaded.
func (s *Server) Reload(paths []string) {
	cmd := cmdCSS
	for _, path := range paths {
		if !strings.HasSuffix(path, ".css") {
			cmd = cmdPage
		}
	}
	s.logger.SayAs("debug", "livereload %s, files changed: %s", cmd, paths)
	s.broadcast <- cmd
}

// Watch montors a channel of lists of paths for reload requests
func (s *Server) Watch(ch chan []string) {
	for ei := range ch {
		if len(ei) > 0 {
			s.Reload(ei)
		}
	}
}

// ServeScript is a handler function that serves the livereload JavaScript file
func (s *Server) ServeScript(rw http.ResponseWriter, req *http.Request) {
	rw.Header().Set("Content-Type", "application/javascript")
	clientBox := rice.MustFindBox("static")
	_, err := rw.Write(clientBox.MustBytes("client.js"))
	if err != nil {
		s.logger.Warn("Error serving livereload script: %s", err)
	}
}
