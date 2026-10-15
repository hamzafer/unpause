package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/hamzafer/unpause/internal/mcpserver"
)

func TestParseSince(t *testing.T) {
	for in, want := range map[string]time.Duration{
		"7d": 7 * 24 * time.Hour, "36h": 36 * time.Hour, "1d12h": 36 * time.Hour, "90m": 90 * time.Minute, "0": 0, "0d": 0,
	} {
		got, err := parseSince(in)
		if err != nil || got != want {
			t.Errorf("parseSince(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if got, err := parseSince("106751d"); err != nil || got <= 0 {
		t.Errorf("parseSince(106751d) = %v, %v; want the largest whole-day window", got, err)
	}
	for _, bad := range []string{"", "x", "7", "d", "-1d", "-2h", "1.5d", "106752d", "106751d24h", "9999999999d"} {
		if _, err := parseSince(bad); err == nil {
			t.Errorf("parseSince(%q) should fail", bad)
		}
	}
}

func TestWriteStats(t *testing.T) {
	st := mcpserver.Stats{
		Calls: 5, Errors: 1,
		First:      time.Date(2026, 10, 14, 10, 0, 0, 0, time.Local),
		Last:       time.Date(2026, 10, 15, 14, 3, 0, 0, time.Local),
		Tools:      []mcpserver.Tally{{Name: "get_session", Count: 3}, {Name: "search_sessions", Count: 2}},
		Clients:    []mcpserver.Tally{{Name: "chatgpt", Count: 5}},
		MostRead:   []mcpserver.Tally{{Name: "11111111-2222-3333-4444-555555555555", Count: 2}, {Name: "deadbeef-0000", Count: 1}},
		TopQueries: []mcpserver.Tally{{Name: "auth bug", Count: 2}},
	}
	lookup := func(id string) (string, string, bool) {
		if strings.HasPrefix(id, "1111") {
			return "personal", "login-work", true
		}
		return "", "", false
	}
	var b bytes.Buffer
	writeStats(&b, statsView(st, lookup), "7d")
	got := b.String()
	for _, want := range []string{
		"calls    5, 2026-10-14 10:00 to 2026-10-15 14:03 (last 7d)",
		"errors   1",
		"  get_session          3",
		"  chatgpt              5",
		"     2  11111111 personal  login-work",
		"     1  deadbeef",
		"     2  auth bug",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// TestWriteStatsLongNames: the name column grows to the longest name in its section (floor
// 18, cap 40), so a real client name like local-agent-mode-unpause shows in full and the
// counts stay in one column. Each section sizes on its own.
func TestWriteStatsLongNames(t *testing.T) {
	long := strings.Repeat("x", 50)
	st := mcpserver.Stats{
		Calls: 7, First: time.Now(), Last: time.Now(),
		Tools:   []mcpserver.Tally{{Name: "get_session", Count: 3}, {Name: long, Count: 1}},
		Clients: []mcpserver.Tally{{Name: "local-agent-mode-unpause", Count: 5}, {Name: "chatgpt", Count: 12}},
	}
	var b bytes.Buffer
	writeStats(&b, statsView(st, func(string) (string, string, bool) { return "", "", false }), "7d")
	got := b.String()
	for _, want := range []string{
		"  local-agent-mode-unpause   5\n",
		"  chatgpt" + strings.Repeat(" ", 24-7) + "  12\n",
		"  get_session" + strings.Repeat(" ", 40-11) + "   3\n",
		"  " + strings.Repeat("x", 39) + "…   1\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// TestWriteStatsStripsControlChars: a query or client name from the log can't drive the
// terminal; control characters (ESC, BEL, C1) never reach the output.
func TestWriteStatsStripsControlChars(t *testing.T) {
	evil := "a\x1b]0;pwned\x07b\x1b[31mred\u009b2Jz\rq"
	st := mcpserver.Stats{
		Calls: 1, First: time.Now(), Last: time.Now(),
		Tools:      []mcpserver.Tally{{Name: evil, Count: 1}},
		Clients:    []mcpserver.Tally{{Name: evil, Count: 1}},
		MostRead:   []mcpserver.Tally{{Name: "\x1b[2J" + evil, Count: 1}, {Name: "11111111-x", Count: 1}},
		TopQueries: []mcpserver.Tally{{Name: evil, Count: 1}},
	}
	lookup := func(id string) (string, string, bool) {
		if strings.HasPrefix(id, "1111") {
			return "acct\x1b[1m", "title\x1b[0m\x07", true
		}
		return "", "", false
	}
	var b bytes.Buffer
	writeStats(&b, statsView(st, lookup), "7d")
	for _, r := range b.String() {
		if r != '\n' && unicode.IsControl(r) {
			t.Fatalf("control char %U in output:\n%q", r, b.String())
		}
	}
	if !strings.Contains(b.String(), "red") {
		t.Errorf("the printable text should stay:\n%s", b.String())
	}
}

// statsHome is an account with one transcript (11111111..., titled login-work) and an
// access log holding a recent call or two and one from long ago.
func statsHome(t *testing.T, lines ...string) []string {
	t.Helper()
	env := emptyAccountEnv(t)
	home := strings.TrimPrefix(env[len(env)-3], "HOME=")
	proj := filepath.Join(home, ".claude", "projects", "-tmp-repo")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile("../../internal/provider/claude/testdata/basic.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "11111111-2222-3333-4444-555555555555.jsonl"), src, 0o644); err != nil {
		t.Fatal(err)
	}
	if lines != nil {
		dir := filepath.Join(home, "cache", "unpause")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "access.jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return append(env, "XDG_CONFIG_HOME="+filepath.Join(home, "xdg"))
}

func TestMCPStatsEmpty(t *testing.T) {
	bin := buildBinary(t)
	cmd := exec.Command(bin, "mcp", "stats")
	cmd.Env = statsHome(t)
	out, err := cmd.CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "no MCP calls logged yet" {
		t.Errorf("empty log: %v %q", err, out)
	}
}

func TestMCPStats(t *testing.T) {
	bin := buildBinary(t)
	now := time.Now().UTC()
	stamp := func(ago time.Duration) string { return now.Add(-ago).Format(time.RFC3339) }
	env := statsHome(t,
		fmt.Sprintf(`{"at":%q,"client":"oldclient","tool":"get_session","input":{"id":"zzzz"},"session_ids":["zzzz9999"],"count":1,"total":1,"duration_ms":1}`, stamp(30*24*time.Hour)),
		fmt.Sprintf(`{"at":%q,"client":"chatgpt","client_version":"1","tool":"search_sessions","input":{"query":"login"},"session_ids":["11111111-2222-3333-4444-555555555555"],"count":1,"total":1,"duration_ms":3}`, stamp(2*time.Hour)),
		fmt.Sprintf(`{"at":%q,"client":"chatgpt","client_version":"1","tool":"get_session","input":{"id":"1111"},"session_ids":["11111111-2222-3333-4444-555555555555"],"count":5,"total":5,"duration_ms":2}`, stamp(time.Hour)),
		fmt.Sprintf(`{"at":%q,"client":"chatgpt","client_version":"1","tool":"get_session","input":{"id":"ffff"},"session_ids":[],"count":0,"total":0,"duration_ms":0,"error":"not found"}`, stamp(time.Minute)),
	)

	cmd := exec.Command(bin, "mcp", "stats")
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("stats: %v\n%s", err, out)
	}
	got := string(out)
	for _, want := range []string{"calls    3,", "(last 7d)", "errors   1", "get_session          2", "chatgpt              3", "11111111 personal  login-work", "login"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "oldclient") || strings.Contains(got, "zzzz") {
		t.Errorf("--since 7d must drop the old call:\n%s", got)
	}

	cmd = exec.Command(bin, "mcp", "stats", "--since", "60d", "--json")
	cmd.Env = env
	out, err = cmd.Output()
	if err != nil {
		t.Fatalf("stats --json: %v\n%s", err, out)
	}
	var js struct {
		Calls    int `json:"calls"`
		Errors   int `json:"errors"`
		MostRead []struct {
			ID      string `json:"id"`
			Count   int    `json:"count"`
			Account string `json:"account"`
			Title   string `json:"title"`
		} `json:"most_read"`
		Clients []mcpserver.Tally `json:"clients"`
	}
	if err := json.Unmarshal(out, &js); err != nil {
		t.Fatalf("decode %s: %v", out, err)
	}
	if js.Calls != 4 || js.Errors != 1 || len(js.MostRead) != 2 || len(js.Clients) != 2 {
		t.Errorf("json: %+v", js)
	}
	if r := js.MostRead[0]; r.ID != "11111111-2222-3333-4444-555555555555" || r.Title != "login-work" || r.Account != "personal" {
		t.Errorf("most read lookup: %+v", r)
	}

	cmd = exec.Command(bin, "mcp", "stats", "--since", "30s")
	cmd.Env = env
	out, err = cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "no MCP calls in the last 30s") {
		t.Errorf("empty window: %v %q", err, out)
	}

	cmd = exec.Command(bin, "mcp", "stats", "--since", "soon")
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Errorf("a bad --since should fail: %s", out)
	}
}
