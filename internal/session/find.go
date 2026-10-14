package session

import "strings"

// Find resolves what a user typed to one Session: an id prefix first, then an exact Name,
// then a substring of the Title. Matching ignores case. It returns nil when nothing matches.
func Find(sessions []*Session, q string) *Session {
	q = strings.ToLower(strings.TrimSpace(q))
	for _, s := range sessions {
		if strings.HasPrefix(strings.ToLower(s.ID), q) {
			return s
		}
	}
	for _, s := range sessions {
		if strings.ToLower(s.CustomTitle) == q {
			return s
		}
	}
	for _, s := range sessions {
		if strings.Contains(strings.ToLower(s.Title()), q) {
			return s
		}
	}
	return nil
}
