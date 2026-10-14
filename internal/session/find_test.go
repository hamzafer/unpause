package session

import "testing"

func TestFind(t *testing.T) {
	ss := []*Session{
		{ID: "aaaa1111-0000", CustomTitle: "Login Work"},
		{ID: "bbbb2222-0000", AITitle: "Fix the parser"},
	}
	cases := map[string]string{
		"AAAA":       "aaaa1111-0000",
		" bbbb2 ":    "bbbb2222-0000",
		"login work": "aaaa1111-0000",
		"parser":     "bbbb2222-0000",
	}
	for q, want := range cases {
		if s := Find(ss, q); s == nil || s.ID != want {
			t.Errorf("Find(%q) = %+v, want %s", q, s, want)
		}
	}
	if Find(ss, "zzz") != nil {
		t.Error("no match must be nil")
	}
}
