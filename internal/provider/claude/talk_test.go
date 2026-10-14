package claude

import (
	"testing"
)

func TestReadTalkKeepsPromptsAndRepliesOnly(t *testing.T) {
	// No thinking, tool calls, tool results, system lines or injected (isMeta) text.
	got, err := ReadTalk("testdata/talk.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	want := []struct{ role, text string }{
		{"user", "Set up the tunnel for the MCP server"},
		{"assistant", "Sure. I'll put the runtime key in the Keychain."},
		{"user", `Also check C:\Users\path please`},
		{"assistant", "Done: the keychain holds the key, the tunnel is up."},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d messages, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Role != w.role || got[i].Text != w.text {
			t.Errorf("message %d = %s %q, want %s %q", i, got[i].Role, got[i].Text, w.role, w.text)
		}
	}
}

func TestReadTalkMissingFile(t *testing.T) {
	if _, err := ReadTalk("testdata/nope.jsonl"); err == nil {
		t.Error("a missing transcript must be an error")
	}
}
