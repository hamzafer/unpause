package session

import (
	"testing"
	"time"
)

func TestWillOfferSummary(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		s    Session
		want bool
	}{
		{"big and idle", Session{LastActive: now.Add(-3 * time.Hour), ContextTokens: 140_000}, true},
		{"big but fresh", Session{LastActive: now.Add(-10 * time.Minute), ContextTokens: 140_000}, false},
		{"idle but small", Session{LastActive: now.Add(-3 * time.Hour), ContextTokens: 60_000}, false},
		{"unknown size", Session{LastActive: now.Add(-3 * time.Hour)}, false},
		{"running now", Session{LastActive: now.Add(-3 * time.Hour), ContextTokens: 140_000, Live: &Live{PID: 1}}, false},
	}
	for _, c := range cases {
		if got := c.s.WillOfferSummary(now); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
	if Tokens(140_503) != "140k" || Tokens(1_250_000) != "1.2M" || Tokens(850) != "850" {
		t.Error("Tokens formatting")
	}
}

func TestTitleOrder(t *testing.T) {
	s := Session{ID: "12345678-x", CustomTitle: "mine", DesktopTitle: "desktop", AITitle: "ai", FirstPrompt: "hi"}
	for _, want := range []string{"mine", "desktop", "ai", "hi", "12345678"} {
		if got := s.Title(); got != want {
			t.Errorf("Title() = %q, want %q", got, want)
		}
		switch want {
		case "mine":
			s.CustomTitle = ""
		case "desktop":
			s.DesktopTitle = ""
		case "ai":
			s.AITitle = ""
		case "hi":
			s.FirstPrompt = ""
		}
	}
}
