package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"code.google.com/p/go.net/websocket"
	"github.com/howeyc/fsnotify"
)

type Config struct {
	port           int
	ignores        string
	ignorePatterns []*regexp.Regexp
	rootDir        string
	reloadJs       *template.Template
	indexHtml      *template.Template
	clients        struct {
		sync.Mutex
		conns []*websocket.Conn
	}
	private   bool
	fsWatcher *fsnotify.Watcher
	delay     float64
}

var config = Config{}

func shouldIgnore(file string) bool {
	for _, p := range config.ignorePatterns {
		if p.Find([]byte(file)) != nil {
			return true
		}
	}

	base := path.Base(file)
	// ignore hidden file and emacs generated file
	if len(base) > 1 && (strings.HasPrefix(base, ".") || strings.HasPrefix(base, "#")) {
		return true
	}

	return false
}

func reloadHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Add("Content-Type", "text/javascript")
	w.Header().Add("Cache-Control", "no-cache")
	config.reloadJs.Execute(w, req.Host)
}

func wshandler(ws *websocket.Conn) {
	config.clients.Lock()
	config.clients.conns = append(config.clients.conns, ws)
	config.clients.Unlock()

	// Wait until the client disconnects.
	// We're not expecting the client to send real data to us
	// so websocket.Read() can be used as a convenient way to block
	ws.Read(nil)
}

func startMonitorFs() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	} else {
		config.fsWatcher = watcher
		walkFn := func(path string, info os.FileInfo, err error) error {
			if err != nil { // TODO permisstion denyed
			}
			ignore := shouldIgnore(path)
			if ignore && info.IsDir() {
				log.Println("ignore dir", path)
				return filepath.SkipDir
			}
			if info.IsDir() && !ignore {
				err = watcher.Watch(path)
				if err != nil {
					log.Fatal(err)
				} else {
					log.Println("monitoring dir", path)
				}
			}
			return nil
		}
		if err := filepath.Walk(config.rootDir, walkFn); err != nil {
			log.Println(err)
		}
	}
}

func compilePatterns() {
	ignores := strings.Split(config.ignores, ",")
	var patterns []*regexp.Regexp
	for _, s := range ignores {
		if len(s) > 0 {
			if p, e := regexp.Compile(s); e == nil {
				patterns = append(patterns, p)
			} else {
				log.Println("ERROR: can not compile to regex", s, e)
			}
		}
	}
	config.ignorePatterns = patterns
}

func notifyBrowsers() {
	defer config.clients.Unlock()
	config.clients.Lock()

	for _, c := range config.clients.conns {
		defer c.Close()
		reload := fmt.Sprintf("%f*1000", config.delay)
		c.Write([]byte(reload))
	}
	config.clients.conns = make([]*websocket.Conn, 0)
}

// remove duplicate, and file name contains #
func cleanEvents(events []*fsnotify.FileEvent) []*fsnotify.FileEvent {
	m := map[string]bool{}
	for _, v := range events {
		if _, seen := m[v.Name]; !seen {
			base := path.Base(v.Name)
			if !strings.Contains(base, "#") {
				events[len(m)] = v
				m[v.Name] = true
			}
		}
	}
	return events[:len(m)]
}

func processFsEvents() {
	var events []*fsnotify.FileEvent
	timer := time.Tick(100 * time.Millisecond)
	for {
		select {
		case <-timer: //  combine events
			if len(events) > 0 {
				events = cleanEvents(events)
				notifyBrowsers()
				events = make([]*fsnotify.FileEvent, 0)
			}
		case ev := <-config.fsWatcher.Event:
			if ev.IsDelete() {
				config.fsWatcher.RemoveWatch(ev.Name)
				events = append(events, ev)
			} else {
				fi, e := os.Lstat(ev.Name)
				if e != nil {
					log.Println(e)
				} else if fi.IsDir() {
					if !shouldIgnore(ev.Name) {
						config.fsWatcher.Watch(ev.Name)
					}
				} else {
					if !shouldIgnore(ev.Name) {
						events = append(events, ev)
					}
				}
			}
		}
	}
}

func main() {
	flag.IntVar(&(config.port), "port", 8000, "Which port to listen")
	flag.StringVar(&(config.rootDir), "rootDir", ".", "Watched root directory for filesystem events, also the HTTP File Server's root directory")
	flag.StringVar(&(config.ignores), "ignores", "", "Ignored file patterns, seprated by ',', used to ignore the filesystem events of some files")
	flag.BoolVar(&(config.private), "private", false, "Only listen on lookback interface, otherwise listen on all interface")
	flag.Float64Var(&(config.delay), "delay", 0, "Delay in seconds before reload browser.")
	flag.Parse()

	config.reloadJs = template.Must(template.New("reloadjs").Parse(RELOADJS))
	config.indexHtml = template.Must(template.New("indexhtml").Parse(INDEXHTML))

	// log.SetFlags(log.LstdFlags | log.Lshortfile)
	compilePatterns()
	err := os.Chdir(config.rootDir)
	if err != nil {
		log.Fatalf("Error changing to root dir '%s', %s\n", config.rootDir, err)
	}

	startMonitorFs()
	go processFsEvents()

	http.HandleFunc("/js", reloadHandler)
	http.Handle("/ws", websocket.Handler(wshandler))
	http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		config.indexHtml.Execute(w, req.Host)
	})

	portStr := ":" + strconv.Itoa(config.port)
	if config.private {
		portStr = "localhost" + portStr
		log.Printf("listens on http://127.0.0.1" + portStr)
	} else {
		log.Printf("listens on http://0.0.0.0" + portStr)
	}
	if err := http.ListenAndServe(portStr, nil); err != nil {
		log.Fatal(err)
	}
}

const INDEXHTML = `
<!DOCTYPE html>
<body>
<h2>http-watcher</h2>
<p>Insert the following snippet into your webpage:
<code>
&lt;script src="http://{{.}}/js"&gt;&lt;/script&gt;
</code>
</p>
</body>
</html>
`

const RELOADJS = `
(function () {
	var added = false;
	function add_js () {
		if(added) { return; }

		if (window.WebSocket){
			var sock = null;
			var wsuri = "ws://{{.}}/ws";

			sock = new WebSocket(wsuri);

			sock.onopen = function() {
				console.log("http-watcher reload connected");
			}
			sock.onclose = function() {
				console.log("http-watcher reload disconnected");
			}

			sock.onmessage = function(e) {
				setTimeout(function() {
					location.reload(true);
				}, parseFloat(e.data));
			}
		} else {
			console.log("http-watch failed, websockets not supported");
		}
		added = true;
	}

	setTimeout(function(){
		setTimeout(add_js, 600);
		window.onload = add_js;
	}, 600)
})();
`
