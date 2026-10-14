package claude

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadMessagesBlocks(t *testing.T) {
	msgs, total, err := ReadMessages("testdata/blocks.jsonl", 0, 100, 20000)
	if err != nil {
		t.Fatal(err)
	}
	// u1, a1, u2, u3, a3: a2 has only an empty text block, the system line is not a message.
	if total != 5 || len(msgs) != 5 {
		t.Fatalf("total=%d len=%d, want 5", total, len(msgs))
	}
	for i, m := range msgs {
		if m.Index != i {
			t.Errorf("msg %d has index %d", i, m.Index)
		}
	}
	if m := msgs[0]; m.Role != "user" || m.At != "2026-09-02T10:00:00Z" || len(m.Blocks) != 1 || m.Blocks[0] != (Block{Type: "text", Text: "Find the Needle in config"}) {
		t.Errorf("string content: %+v", m)
	}
	a := msgs[1]
	if a.Role != "assistant" || len(a.Blocks) != 3 {
		t.Fatalf("assistant blocks: %+v", a.Blocks)
	}
	if a.Blocks[0].Type != "thinking" || a.Blocks[0].Text != "where is it" {
		t.Errorf("thinking: %+v", a.Blocks[0])
	}
	if a.Blocks[1].Type != "text" || a.Blocks[1].Text != "Looking now." {
		t.Errorf("text: %+v", a.Blocks[1])
	}
	tu := a.Blocks[2]
	if tu.Type != "tool_use" || tu.Name != "Grep" {
		t.Errorf("tool_use: %+v", tu)
	}
	in, _ := json.Marshal(tu.Input)
	if string(in) != `{"pattern":"needle","path":"/tmp/repo"}` {
		t.Errorf("tool_use input = %s", in)
	}
	if b := msgs[2].Blocks; len(b) != 1 || b[0].Type != "tool_result" || b[0].Text != "config.go:3: NEEDLE = 1\nconfig.go:9: needle2" || b[0].IsError {
		t.Errorf("tool_result array: %+v", b)
	}
	if b := msgs[3].Blocks; len(b) != 1 || b[0].Text != "permission denied" || !b[0].IsError {
		t.Errorf("tool_result error: %+v", b)
	}
	if msgs[4].Blocks[0].Text != "It says \"quoted\" and\nspans lines." {
		t.Errorf("a3: %+v", msgs[4])
	}
}

func TestReadMessagesPaging(t *testing.T) {
	msgs, total, err := ReadMessages("testdata/blocks.jsonl", 3, 10, 20000)
	if err != nil {
		t.Fatal(err)
	}
	if total != 5 || len(msgs) != 2 || msgs[0].Index != 3 || msgs[1].Index != 4 {
		t.Errorf("total=%d msgs=%+v", total, msgs)
	}
	msgs, total, _ = ReadMessages("testdata/blocks.jsonl", 1, 1, 20000)
	if total != 5 || len(msgs) != 1 || msgs[0].Index != 1 {
		t.Errorf("limit 1: total=%d msgs=%+v", total, msgs)
	}
	msgs, total, _ = ReadMessages("testdata/blocks.jsonl", 50, 10, 20000)
	if total != 5 || len(msgs) != 0 {
		t.Errorf("past the end: total=%d msgs=%+v", total, msgs)
	}
	// offset+limit overflows here; the page must still be empty, not everything.
	msgs, total, _ = ReadMessages("testdata/blocks.jsonl", math.MaxInt, 50, 20000)
	if total != 5 || len(msgs) != 0 {
		t.Errorf("offset MaxInt: total=%d msgs=%+v", total, msgs)
	}
	msgs, _, _ = ReadMessages("testdata/blocks.jsonl", 1, math.MaxInt, 20000)
	if len(msgs) != 4 || msgs[0].Index != 1 {
		t.Errorf("limit MaxInt: msgs=%+v", msgs)
	}
}

func TestReadMessagesTruncates(t *testing.T) {
	msgs, _, err := ReadMessages("testdata/blocks.jsonl", 0, 100, 10)
	if err != nil {
		t.Fatal(err)
	}
	if got := msgs[0].Blocks[0].Text; got != "Find the N[… 15 more chars]" {
		t.Errorf("text cut = %q", got)
	}
	in, ok := msgs[1].Blocks[2].Input.(string)
	if !ok || !strings.HasPrefix(in, `{"pattern"`) || !strings.HasSuffix(in, " more chars]") {
		t.Errorf("long tool input should become a cut string, got %#v", msgs[1].Blocks[2].Input)
	}
	// Rune-safe: never split a multi-byte character.
	if got := Truncate("ééééé", 2); got != "éé[… 3 more chars]" {
		t.Errorf("rune cut = %q", got)
	}
}

func TestReadMessagesHugeLine(t *testing.T) {
	p := filepath.Join(t.TempDir(), "big.jsonl")
	big := strings.Repeat("x", 3<<20)
	line := `{"type":"user","message":{"role":"user","content":"` + big + `"},"timestamp":"2026-09-02T10:00:00.000Z"}` + "\n"
	if err := os.WriteFile(p, []byte(line+line), 0o644); err != nil {
		t.Fatal(err)
	}
	msgs, total, err := ReadMessages(p, 0, 10, 100)
	if err != nil || total != 2 || !strings.HasSuffix(msgs[1].Blocks[0].Text, "[… 3145628 more chars]") {
		t.Fatalf("err=%v total=%d", err, total)
	}
}

func TestSearchFile(t *testing.T) {
	m := NewMatcher("needle")
	r, err := m.SearchFile(context.Background(), "testdata/blocks.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	// "Needle" (prompt), "needle" (tool input), "NEEDLE" and "needle2" (tool result).
	// "where is it" has none; the system line is not a message.
	if r.Hits != 4 {
		t.Errorf("hits = %d, want 4", r.Hits)
	}
	if len(r.Snippets) != 3 || !strings.Contains(r.Snippets[0], "Find the Needle in config") {
		t.Errorf("snippets = %q", r.Snippets)
	}

	r, _ = NewMatcher(`says "quoted" and`).SearchFile(context.Background(), "testdata/blocks.jsonl")
	if r.Hits != 1 {
		t.Errorf("query with quotes: hits = %d, want 1", r.Hits)
	}
	r, _ = NewMatcher("and\nspans").SearchFile(context.Background(), "testdata/blocks.jsonl")
	if r.Hits != 1 {
		t.Errorf("query with newline: hits = %d, want 1", r.Hits)
	}
	// A tool's name counts, so a session that only called Grep is found by "grep".
	r, _ = NewMatcher("grep").SearchFile(context.Background(), "testdata/blocks.jsonl")
	if r.Hits != 1 || len(r.Snippets) != 1 || r.Snippets[0] != "Grep" {
		t.Errorf("tool name: %+v", r)
	}
	r, _ = NewMatcher("nowhere-to-be-found").SearchFile(context.Background(), "testdata/blocks.jsonl")
	if r.Hits != 0 || len(r.Snippets) != 0 {
		t.Errorf("miss: %+v", r)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.SearchFile(ctx, "testdata/blocks.jsonl"); err == nil {
		t.Error("cancelled context must stop the scan")
	}
}

func TestSnippetWindow(t *testing.T) {
	text := strings.Repeat("a ", 200) + "TARGET" + strings.Repeat(" b", 200)
	m := NewMatcher("target")
	r := &SearchResult{}
	m.match(text, r)
	if r.Hits != 1 || len(r.Snippets) != 1 {
		t.Fatalf("%+v", r)
	}
	s := r.Snippets[0]
	if !strings.Contains(s, "TARGET") || len([]rune(s)) > 210 || !strings.HasPrefix(s, "…") || !strings.HasSuffix(s, "…") {
		t.Errorf("snippet = %q (%d runes)", s, len([]rune(s)))
	}
}
