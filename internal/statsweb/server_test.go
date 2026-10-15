package statsweb

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/hamzafer/unpause/internal/mcpserver"
)

const port = 4777

const token = "0123456789abcdef0123456789abcdef"

// withToken adds the run's token to an API target.
func withToken(target string) string {
	if strings.Contains(target, "?") {
		return target + "&t=" + token
	}
	return target + "?t=" + token
}

func testConfig(es []mcpserver.Entry, readErr error) Config {
	return Config{
		ReadLog: func() ([]mcpserver.Entry, error) { return es, readErr },
		Lookup: func() Lookup {
			return func(id string) (Info, bool) {
				if id == "s1-aaaa-bbbb" {
					return Info{Title: "T", Account: "work", Project: "p"}, true
				}
				return Info{}, false
			}
		},
		Now:   func() time.Time { return now },
		Loc:   zone,
		Token: token,
		Static: fstest.MapFS{
			"index.html":             {Data: []byte("<!doctype html><p>stats</p>")},
			"404.html":               {Data: []byte("<!doctype html><p>gone</p>")},
			"about.html":             {Data: []byte("<!doctype html><p>about</p>")},
			"_next/static/app.js":    {Data: []byte("console.log(1)")},
			"_next/static/app.css":   {Data: []byte("body{}")},
			"_next/static/g.woff2":   {Data: []byte("wOF2")},
			"index.txt":              {Data: []byte("rsc")},
			"favicon.ico":            {Data: []byte{0, 0, 1, 0}},
			"_next/static/data.json": {Data: []byte("{}")},
		},
	}
}

func do(t *testing.T, h http.Handler, method, target, host string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, target, nil)
	r.Host = host
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

var okHost = "127.0.0.1:" + strconv.Itoa(port)

func TestHostCheck(t *testing.T) {
	h := NewHandler(testConfig(nil, nil), port)
	for host, want := range map[string]int{
		"127.0.0.1:4777":  200,
		"localhost:4777":  200,
		"LocalHost:4777":  200,
		"evil.com:4777":   403,
		"evil.com":        403,
		"127.0.0.1":       403,
		"127.0.0.1:4778":  403,
		"localhost":       403,
		"[::1]:4777":      403,
		"127.0.0.1.:4777": 403,
		"":                403,
	} {
		for _, path := range []string{"/", withToken("/api/stats?range=24h")} {
			if got := do(t, h, "GET", path, host).Code; got != want {
				t.Errorf("Host %q %s: %d, want %d", host, path, got, want)
			}
		}
	}
}

func TestMethods(t *testing.T) {
	h := NewHandler(testConfig(nil, nil), port)
	for _, m := range []string{"POST", "PUT", "DELETE", "PATCH", "OPTIONS"} {
		w := do(t, h, m, withToken("/api/stats?range=24h"), okHost)
		if w.Code != 405 || w.Header().Get("Allow") != "GET, HEAD" {
			t.Errorf("%s: %d Allow=%q", m, w.Code, w.Header().Get("Allow"))
		}
	}
	w := do(t, h, "HEAD", "/", okHost)
	if w.Code != 200 || w.Body.Len() != 0 {
		t.Errorf("HEAD /: %d, %d bytes", w.Code, w.Body.Len())
	}
}

func TestStatsEndpoint(t *testing.T) {
	es := entries(ent{ago: time.Hour, client: "codex", tool: "get_session", ids: []string{"s1-aaaa-bbbb"}, dur: 5})
	h := NewHandler(testConfig(es, nil), port)

	w := do(t, h, "GET", withToken("/api/stats?range=24h"), okHost)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content type %q", ct)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("cache control %q", cc)
	}
	for k := range w.Header() {
		if strings.HasPrefix(strings.ToLower(k), "access-control-") {
			t.Errorf("no CORS headers, got %s", k)
		}
	}
	var st Stats
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil || st.KPIs.Calls != 1 || st.MostRead[0].Title != "T" {
		t.Errorf("%v %+v", err, st)
	}

	// no range means 7d
	w = do(t, h, "GET", withToken("/api/stats"), okHost)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"range":"7d"`) {
		t.Errorf("default range: %d %s", w.Code, w.Body)
	}

	for _, bad := range []string{"bogus", "1h", "ALL"} {
		w = do(t, h, "GET", withToken("/api/stats?range="+bad), okHost)
		if w.Code != 400 || w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Body.String(), `"error"`) {
			t.Errorf("range %q: %d %s", bad, w.Code, w.Body)
		}
	}

	w = do(t, h, "GET", withToken("/api/nope"), okHost)
	if w.Code != 404 || w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("unknown api: %d %q", w.Code, w.Header().Get("Content-Type"))
	}

	h = NewHandler(testConfig(nil, errors.New("disk on fire")), port)
	w = do(t, h, "GET", withToken("/api/stats?range=24h"), okHost)
	if w.Code != 500 || strings.Contains(w.Body.String(), "disk on fire") {
		t.Errorf("read error: %d %s", w.Code, w.Body)
	}
}

func TestSessionEndpoint(t *testing.T) {
	es := entries(ent{ago: time.Hour, client: "codex", tool: "get_session", ids: []string{"s1-aaaa-bbbb"}})
	h := NewHandler(testConfig(es, nil), port)
	w := do(t, h, "GET", withToken("/api/session?id=s1-aaaa-bbbb"), okHost)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("%d %v", w.Code, w.Header())
	}
	var s Session
	if err := json.Unmarshal(w.Body.Bytes(), &s); err != nil || s.Title != "T" || len(s.Calls) != 1 || s.Calls[0].ClientName != "Codex" {
		t.Errorf("%v %+v", err, s)
	}
	for _, q := range []string{"?id=unknown", "?id=s1", ""} {
		w = do(t, h, "GET", withToken("/api/session"+q), okHost)
		if w.Code != 404 || w.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("session%s: %d", q, w.Code)
		}
	}
}

func TestStaticFiles(t *testing.T) {
	h := NewHandler(testConfig(nil, nil), port)
	for path, want := range map[string]string{
		"/":                       "text/html; charset=utf-8",
		"/index.html":             "text/html; charset=utf-8",
		"/about":                  "text/html; charset=utf-8",
		"/_next/static/app.js":    "text/javascript; charset=utf-8",
		"/_next/static/app.css":   "text/css; charset=utf-8",
		"/_next/static/g.woff2":   "font/woff2",
		"/_next/static/data.json": "application/json",
		"/index.txt":              "text/plain; charset=utf-8",
		"/favicon.ico":            "image/x-icon",
	} {
		w := do(t, h, "GET", path, okHost)
		if w.Code != 200 || w.Header().Get("Content-Type") != want {
			t.Errorf("%s: %d %q, want %q", path, w.Code, w.Header().Get("Content-Type"), want)
		}
		if w.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: no nosniff", path)
		}
	}
	if b := do(t, h, "GET", "/", okHost).Body.String(); !strings.Contains(b, "stats") {
		t.Errorf("/ should serve index.html: %q", b)
	}
	for _, path := range []string{"/missing.js", "/_next/", "/_next/static", "/../etc/passwd", "/nope"} {
		w := do(t, h, "GET", path, okHost)
		if w.Code != 404 {
			t.Errorf("%s: %d, want 404", path, w.Code)
		}
	}
	if b := do(t, h, "GET", "/nope", okHost).Body.String(); !strings.Contains(b, "gone") {
		t.Errorf("404 should use the export's 404.html: %q", b)
	}
}

func TestListenFallsBackWhenBusy(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	taken := busy.Addr().(*net.TCPAddr).Port
	ln, err := Listen(taken)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	a := ln.Addr().(*net.TCPAddr)
	if a.Port == taken || !a.IP.Equal(net.IPv4(127, 0, 0, 1)) {
		t.Errorf("got %v, want 127.0.0.1 on another port than %d", a, taken)
	}
}

func TestServeAndShutdown(t *testing.T) {
	ln, err := Listen(0)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, ln, testConfig(nil, nil)) }()

	url := "http://" + ln.Addr().String() + withToken("/api/stats?range=all")
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(string(body), `"empty":true`) {
		t.Errorf("%d %s", res.StatusCode, body)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Serve: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve didn't stop")
	}
}

func TestTokenGuardsTheAPI(t *testing.T) {
	h := NewHandler(testConfig(nil, nil), port)
	for _, target := range []string{
		"/api/stats?range=24h",
		"/api/stats?range=24h&t=",
		"/api/stats?range=24h&t=" + strings.ToUpper(token),
		"/api/stats?range=24h&t=" + token[:31],
		"/api/stats?range=24h&t=" + token + "0",
		"/api/session?id=s1-aaaa-bbbb",
		"/api/nope",
	} {
		w := do(t, h, "GET", target, okHost)
		if w.Code != 403 || w.Header().Get("Content-Type") != "application/json" || strings.TrimSpace(w.Body.String()) != `{"error":"forbidden"}` {
			t.Errorf("%s: %d %q %s", target, w.Code, w.Header().Get("Content-Type"), w.Body)
		}
	}
	if w := do(t, h, "GET", withToken("/api/stats?range=24h"), okHost); w.Code != 200 {
		t.Errorf("with the token: %d", w.Code)
	}
	// The page itself needs no token: it reads it from its own URL.
	for _, target := range []string{"/", "/?t=" + token, "/_next/static/app.js"} {
		if w := do(t, h, "GET", target, okHost); w.Code != 200 {
			t.Errorf("%s: %d", target, w.Code)
		}
	}
	// A server without a token refuses every API call.
	cfg := testConfig(nil, nil)
	cfg.Token = ""
	if w := do(t, NewHandler(cfg, port), "GET", "/api/stats?t=", okHost); w.Code != 403 {
		t.Errorf("no token configured: %d", w.Code)
	}
}

func TestNewToken(t *testing.T) {
	a, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := NewToken()
	if len(a) != 32 || a == b || strings.Trim(a, "0123456789abcdef") != "" {
		t.Errorf("tokens %q %q: want two different 32-digit hex strings", a, b)
	}
}

// An absolute-form target (GET http://host/path) sets r.Host from the URL, not the Host
// header, so it is refused before the host check.
func TestAbsoluteFormRefused(t *testing.T) {
	ln, err := Listen(0)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, ln, testConfig(nil, nil)) }()
	defer func() { cancel(); <-done }()

	addr := ln.Addr().String()
	for _, target := range []string{
		"http://" + addr + withToken("/api/stats"),
		"http://" + addr + "/",
	} {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: evil.com\r\nConnection: close\r\n\r\n", target)
		res, err := http.ReadResponse(bufio.NewReader(conn), nil)
		conn.Close()
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != 400 {
			t.Errorf("%s: %d, want 400", target, res.StatusCode)
		}
	}
}

// The title index is loaded at most once per titleTTL; the log is read on every request.
func TestTitleLookupCached(t *testing.T) {
	es := entries(ent{ago: time.Hour, client: "codex", tool: "get_session", ids: []string{"s1-aaaa-bbbb"}})
	cfg := testConfig(es, nil)
	var reads, loads int
	var mu sync.Mutex
	read := cfg.ReadLog
	cfg.ReadLog = func() ([]mcpserver.Entry, error) { mu.Lock(); reads++; mu.Unlock(); return read() }
	lookup := cfg.Lookup
	cfg.Lookup = func() Lookup { mu.Lock(); loads++; mu.Unlock(); return lookup() }
	clock := now
	cfg.Now = func() time.Time { mu.Lock(); defer mu.Unlock(); return clock }
	h := NewHandler(cfg, port)

	get := func(target string) {
		t.Helper()
		if w := do(t, h, "GET", withToken(target), okHost); w.Code != 200 || !strings.Contains(w.Body.String(), `"T"`) {
			t.Fatalf("%s: %d %s", target, w.Code, w.Body)
		}
	}
	get("/api/stats?range=24h")
	mu.Lock()
	clock = clock.Add(20 * time.Second)
	mu.Unlock()
	get("/api/stats?range=24h")
	get("/api/session?id=s1-aaaa-bbbb")
	if reads != 3 || loads != 1 {
		t.Errorf("within 30 s: %d log reads (want 3), %d index loads (want 1)", reads, loads)
	}
	mu.Lock()
	clock = clock.Add(11 * time.Second)
	mu.Unlock()
	get("/api/stats?range=24h")
	if loads != 2 {
		t.Errorf("after 30 s: %d index loads, want 2", loads)
	}

	// Concurrent requests share one rebuild.
	mu.Lock()
	clock = clock.Add(time.Minute)
	mu.Unlock()
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			do(t, h, "GET", withToken("/api/stats?range=24h"), okHost)
		}()
	}
	wg.Wait()
	if loads != 3 {
		t.Errorf("8 concurrent requests: %d index loads, want 3 in total", loads)
	}
}

// A connection still sending its request when the server stops doesn't make Serve fail
// with an error: it reports ErrSlowShutdown after the grace period and closes it.
func TestServeSlowShutdown(t *testing.T) {
	old := shutdownGrace
	shutdownGrace = 100 * time.Millisecond
	defer func() { shutdownGrace = old }()

	ln, err := Listen(0)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, ln, testConfig(nil, nil)) }()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	fmt.Fprint(conn, "GET / HTTP/1.1\r\n") // headers never finish
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, ErrSlowShutdown) {
			t.Errorf("Serve: %v, want ErrSlowShutdown", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve didn't stop")
	}
}
