package recap

import (
	"os"

	"github.com/hamzafer/unpause/internal/session"
)

// Outdated reports whether s's transcript has grown since r was made from it. A
// transcript that can't be read is not outdated: there is nothing to recap again.
func Outdated(r Recap, s *session.Session) bool {
	st, err := os.Stat(s.Path)
	return err == nil && st.Size() > r.Size
}
