package recap

import (
	"os"

	"github.com/hamzafer/unpause/internal/session"
)

// Outdated reports whether r was made from a different transcript than s has now: the
// transcript's size changed since. A transcript that can't be read says nothing, so the
// Recap is not called outdated.
func Outdated(r Recap, s *session.Session) bool {
	st, err := os.Stat(s.Path)
	if err != nil {
		return false
	}
	return st.Size() != r.Size
}
