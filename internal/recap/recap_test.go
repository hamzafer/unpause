package recap

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hamzafer/unpause/internal/session"
)

func padelSession() *session.Session {
	return &session.Session{ID: "11111111-aaaa", CWD: "/x/padel", Branch: "main", AITitle: "Padel in Oslo", Messages: 4}
}

func TestInputCarriesHeaderAndTalk(t *testing.T) {
	talk := []session.Message{
		{Role: "user", Text: "find the cheapest padel for 4 in Oslo"},
		{Role: "assistant", Text: "Bærum and Just Padel have showers"},
	}
	in := Input(padelSession(), talk)
	for _, want := range []string{"Repo: padel", "Branch: main", "Title: Padel in Oslo", "find the cheapest padel for 4 in Oslo", "Bærum and Just Padel have showers"} {
		if !strings.Contains(in, want) {
			t.Errorf("input missing %q:\n%s", want, in)
		}
	}
	if strings.Index(in, "cheapest") > strings.Index(in, "Bærum") {
		t.Errorf("talk must stay in file order:\n%s", in)
	}
}

func TestInputIsBoundedAndKeepsFirstPromptAndEnd(t *testing.T) {
	talk := []session.Message{{Role: "user", Text: "FIRST PROMPT " + strings.Repeat("a", 5000)}}
	for i := 0; i < 200; i++ {
		talk = append(talk, session.Message{Role: "assistant", Text: fmt.Sprintf("reply %03d %s", i, strings.Repeat("ø", 300))})
	}
	talk = append(talk, session.Message{Role: "user", Text: "LAST PROMPT"})
	in := Input(padelSession(), talk)
	if n := utf8.RuneCountInString(in); n > MaxInput {
		t.Errorf("input is %d characters, want at most %d", n, MaxInput)
	}
	for _, want := range []string{"FIRST PROMPT", "LAST PROMPT", "reply 199"} {
		if !strings.Contains(in, want) {
			t.Errorf("input missing %q", want)
		}
	}
	if strings.Contains(in, "reply 000") {
		t.Error("the middle should be dropped when the talk doesn't fit")
	}
	if !utf8.ValidString(in) {
		t.Error("cutting must keep runes whole")
	}
}

func TestParseGoodReply(t *testing.T) {
	r, err := Parse("goal  cheapest padel for 4 in Oslo, 21:00-24:00\ndone  top 3 found; showers confirmed\nnext  decide between a membership and paying per game\n")
	if err != nil {
		t.Fatal(err)
	}
	if r.Goal != "cheapest padel for 4 in Oslo, 21:00-24:00" || r.Done != "top 3 found; showers confirmed" || r.Next != "decide between a membership and paying per game" {
		t.Errorf("got %+v", r)
	}
}

func TestParseToleratesChatterAndMarkdown(t *testing.T) {
	r, err := Parse("Here is the recap:\n\n- **Goal:** ship the hook\n* Done: tests pass\nNEXT: open the PR\n\nLet me know!")
	if err != nil {
		t.Fatal(err)
	}
	if r.Goal != "ship the hook" || r.Done != "tests pass" || r.Next != "open the PR" {
		t.Errorf("got %+v", r)
	}
}

func TestParseRejectsBadReplies(t *testing.T) {
	long := strings.Repeat("word ", 40)
	for name, reply := range map[string]string{
		"missing next":   "goal a\ndone b\n",
		"empty value":    "goal a\ndone\nnext c",
		"duplicate goal": "goal a\ngoal b\ndone c\nnext d",
		"too long":       "goal " + long + "\ndone b\nnext c",
		"just talk":      "I can't summarize this session, it has too little in it.",
		"label in prose": "the goal was to fix it\ndone b\nnext c",
	} {
		if r, err := Parse(reply); err == nil {
			t.Errorf("%s: want an error, got %+v", name, r)
		}
	}
}
