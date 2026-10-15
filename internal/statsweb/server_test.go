package statsweb

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/hamzafer/unpause/internal/mcpserver"
)

const port = 4777

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
		Now: func() time.Time { return now },
		Loc: zone,
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
		for _, path := range []string{"/", "/api/stats?range=24h"} {
			if got := do(t, h, "GET", path, host).Code; got != want {
				t.Errorf("Host %q %s: %d, want %d", host, path, got, want)
			}
		}
	}
}

func TestMethods(t *testing.T) {
	h := NewHandler(testConfig(nil, nil), port)
	for _, m := range []string{"POST", "PUT", "DELETE", "PATCH", "OPTIONS"} {
		w := do(t, h, m, "/api/stats?range=24h", okHost)
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

	w := do(t, h, "GET", "/api/stats?range=24h", okHost)
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
	w = do(t, h, "GET", "/api/stats", okHost)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"range":"7d"`) {
		t.Errorf("default range: %d %s", w.Code, w.Body)
	}

	for _, bad := range []string{"bogus", "1h", "ALL"} {
		w = do(t, h, "GET", "/api/stats?range="+bad, okHost)
		if w.Code != 400 || w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Body.String(), `"error"`) {
			t.Errorf("range %q: %d %s", bad, w.Code, w.Body)
		}
	}

	w = do(t, h, "GET", "/api/nope", okHost)
	if w.Code != 404 || w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("unknown api: %d %q", w.Code, w.Header().Get("Content-Type"))
	}

	h = NewHandler(testConfig(nil, errors.New("disk on fire")), port)
	w = do(t, h, "GET", "/api/stats?range=24h", okHost)
	if w.Code != 500 || strings.Contains(w.Body.String(), "disk on fire") {
		t.Errorf("read error: %d %s", w.Code, w.Body)
	}
}

func TestSessionEndpoint(t *testing.T) {
	es := entries(ent{ago: time.Hour, client: "codex", tool: "get_session", ids: []string{"s1-aaaa-bbbb"}})
	h := NewHandler(testConfig(es, nil), port)
	w := do(t, h, "GET", "/api/session?id=s1-aaaa-bbbb", okHost)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("%d %v", w.Code, w.Header())
	}
	var s Session
	if err := json.Unmarshal(w.Body.Bytes(), &s); err != nil || s.Title != "T" || len(s.Calls) != 1 || s.Calls[0].ClientName != "Codex" {
		t.Errorf("%v %+v", err, s)
	}
	for _, q := range []string{"?id=unknown", "?id=s1", ""} {
		w = do(t, h, "GET", "/api/session"+q, okHost)
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

	url := "http://" + ln.Addr().String() + "/api/stats?range=all"
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
