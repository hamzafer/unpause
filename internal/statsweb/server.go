package statsweb

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hamzafer/unpause/internal/mcpserver"
)

// Config is what the server reads. Every API request calls ReadLog again, so the counts
// are as fresh as `unpause mcp stats`. Lookup loads the index for titles only when a
// response names Sessions, and its result is kept for titleTTL.
type Config struct {
	ReadLog func() ([]mcpserver.Entry, error)
	Lookup  func() Lookup // loads the index; at most once per titleTTL
	Now     func() time.Time
	Loc     *time.Location // bucket boundaries; time.Local in the command
	Static  fs.FS          // the export: index.html, _next/...
	// Token is this run's secret. Every /api/ request has to carry it as ?t=; without
	// one, every API request is refused. The page gets it from the URL the command prints.
	Token string
}

// NewToken is a random 128-bit token in hex, one per run of the server.
func NewToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// titleTTL is how long a loaded index serves titles before the next request reloads it.
const titleTTL = 30 * time.Second

// titles keeps the loaded index for titleTTL. One load at a time: a request that needs
// titles while another loads them waits for that load.
type titles struct {
	mu     sync.Mutex
	load   func() Lookup
	now    func() time.Time
	at     time.Time
	loaded bool
	l      Lookup
}

func (t *titles) get() Lookup {
	t.mu.Lock()
	defer t.mu.Unlock()
	if n := t.now(); !t.loaded || n.Sub(t.at) >= titleTTL || n.Before(t.at) {
		t.l, t.at, t.loaded = t.load(), n, true
	}
	return t.l
}

// lookup loads the titles on its first call only, so a response that names no Session
// never touches the index.
func (t *titles) lookup() Lookup {
	var once sync.Once
	var l Lookup
	return func(id string) (Info, bool) {
		once.Do(func() { l = t.get() })
		if l == nil {
			return Info{}, false
		}
		return l(id)
	}
}

// ErrSlowShutdown is what Serve returns when connections were still open after the
// grace period and had to be closed.
var ErrSlowShutdown = errors.New("connections still open on stop; closed them")

// shutdownGrace is how long Serve waits for open requests when ctx is done.
var shutdownGrace = 5 * time.Second

// Listen binds 127.0.0.1:port, or a free port the OS picks when that one is taken.
// Port 0 always lets the OS pick.
func Listen(port int) (net.Listener, error) {
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if errors.Is(err, syscall.EADDRINUSE) {
		return net.Listen("tcp", "127.0.0.1:0")
	}
	return ln, err
}

// Serve answers on ln until ctx is done, then shuts down gracefully.
func Serve(ctx context.Context, ln net.Listener, cfg Config) error {
	srv := &http.Server{
		Handler:           NewHandler(cfg, ln.Addr().(*net.TCPAddr).Port),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	sctx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		srv.Close()
		<-errc
		if errors.Is(err, context.DeadlineExceeded) {
			return ErrSlowShutdown
		}
		return err
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// NewHandler serves the page and the API for a server bound to 127.0.0.1:port. It answers
// only requests addressed to that host and port, so a page on another site can't read the
// log through DNS rebinding, sends no CORS headers, and answers GET and HEAD only. The API
// also wants the run's token, so another local user or process can't read titles without
// the URL the command printed.
func NewHandler(cfg Config, port int) http.Handler {
	p := strconv.Itoa(port)
	s := &server{Config: cfg, titles: &titles{load: cfg.Lookup, now: cfg.Now}}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		// An absolute-form target (GET http://host/path) makes r.Host the URL's host, not
		// the Host header's, so the host check below can't be trusted with one.
		if !strings.HasPrefix(r.RequestURI, "/") {
			http.Error(w, "bad request target", http.StatusBadRequest)
			return
		}
		if r.Host != "127.0.0.1:"+p && !strings.EqualFold(r.Host, "localhost:"+p) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			h.Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		p := r.URL.Path
		if strings.HasPrefix(p, "/api/") && !s.authorized(r) {
			writeJSON(w, http.StatusForbidden, apiError{"forbidden"})
			return
		}
		switch {
		case p == "/api/stats":
			s.stats(w, r)
		case p == "/api/session":
			s.session(w, r)
		case strings.HasPrefix(p, "/api/"):
			writeJSON(w, http.StatusNotFound, apiError{"not found"})
		default:
			s.static(w, r)
		}
	})
}

// server is a Config plus the state one handler keeps across requests.
type server struct {
	Config
	titles *titles
}

// authorized reports whether r carries the run's token as ?t=, compared in constant time.
func (s *server) authorized(r *http.Request) bool {
	got := r.URL.Query().Get("t")
	return s.Token != "" && subtle.ConstantTimeCompare([]byte(got), []byte(s.Token)) == 1
}

type apiError struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		status, b = http.StatusInternalServerError, []byte(`{"error":"internal error"}`)
	}
	h := w.Header()
	h.Set("Content-Type", "application/json")
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Length", strconv.Itoa(len(b)))
	w.WriteHeader(status)
	w.Write(b)
}

func (c Config) read(w http.ResponseWriter) ([]mcpserver.Entry, bool) {
	es, err := c.ReadLog()
	if err != nil {
		// The error text can name paths; the page only needs to know it failed.
		writeJSON(w, http.StatusInternalServerError, apiError{"could not read the access log"})
		return nil, false
	}
	return es, true
}

func (c *server) stats(w http.ResponseWriter, r *http.Request) {
	rng := r.URL.Query().Get("range")
	if rng == "" {
		rng = "7d"
	}
	if _, ok := spans[rng]; !ok {
		writeJSON(w, http.StatusBadRequest, apiError{ErrRange.Error()})
		return
	}
	es, ok := c.read(w)
	if !ok {
		return
	}
	st, err := BuildStats(es, rng, c.Now(), c.Loc, c.titles.lookup())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (c *server) session(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	es, ok := c.read(w)
	if !ok {
		return
	}
	s, found := BuildSession(es, id, c.Now(), c.titles.lookup())
	if id == "" || !found {
		writeJSON(w, http.StatusNotFound, apiError{"no logged call returned this session"})
		return
	}
	writeJSON(w, http.StatusOK, s)
}

// contentTypes covers what a Next.js export holds, so the type doesn't depend on the
// machine's mime tables.
var contentTypes = map[string]string{
	".html":  "text/html; charset=utf-8",
	".js":    "text/javascript; charset=utf-8",
	".mjs":   "text/javascript; charset=utf-8",
	".css":   "text/css; charset=utf-8",
	".json":  "application/json",
	".txt":   "text/plain; charset=utf-8",
	".map":   "application/json",
	".svg":   "image/svg+xml",
	".png":   "image/png",
	".jpg":   "image/jpeg",
	".webp":  "image/webp",
	".ico":   "image/x-icon",
	".woff":  "font/woff",
	".woff2": "font/woff2",
	".ttf":   "font/ttf",
}

func contentType(name string) string {
	ext := strings.ToLower(path.Ext(name))
	if ct, ok := contentTypes[ext]; ok {
		return ct
	}
	if ct := mime.TypeByExtension(ext); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

// static serves the export: "/" is index.html, "/about" may be about.html, a folder is
// its index.html or nothing (no listings), anything else missing is the export's 404.html.
func (c Config) static(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == "" {
		name = "index.html"
	}
	candidates := []string{name}
	if path.Ext(name) == "" {
		candidates = append(candidates, name+"/index.html", name+".html")
	}
	for _, n := range candidates {
		if fi, err := fs.Stat(c.Static, n); err == nil && fi.Mode().IsRegular() {
			c.serveFile(w, r, n, http.StatusOK)
			return
		}
	}
	if fi, err := fs.Stat(c.Static, "404.html"); err == nil && fi.Mode().IsRegular() {
		c.serveFile(w, r, "404.html", http.StatusNotFound)
		return
	}
	http.NotFound(w, r)
}

func (c Config) serveFile(w http.ResponseWriter, r *http.Request, name string, status int) {
	b, err := fs.ReadFile(c.Static, name)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", contentType(name))
	if status != http.StatusOK {
		w.Header().Set("Content-Length", strconv.Itoa(len(b)))
		w.WriteHeader(status)
		if r.Method != http.MethodHead {
			w.Write(b)
		}
		return
	}
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(b))
}
