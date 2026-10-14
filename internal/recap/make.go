package recap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/hamzafer/unpause/internal/provider/claude"
	"github.com/hamzafer/unpause/internal/session"
)

// SystemPrompt tells the model what a Recap looks like.
const SystemPrompt = "You write a three-line recap of a coding session for a session picker. " +
	"Reply with exactly three lines and nothing else:\n" +
	"goal  what the person was trying to do\n" +
	"done  what got done so far\n" +
	"next  the next step left open\n" +
	"Each line starts with its label, is plain text and at most 100 characters."

// Model asks a model about a Session, given the input Input built. The reply may be
// messy; Make parses it.
type Model func(ctx context.Context, s *session.Session, input string) (string, error)

// ErrLive is Can's reason for a Live Session: it is unfinished, so a Recap would be
// outdated as soon as it was written.
var ErrLive = errors.New("session is still running: no Recap until it ends")

// Can reports why s can't have a Recap, or nil when it can: Live Sessions are unfinished,
// Cowork Sessions belong to a read-only Account, and sidechains and empty Sessions have
// nothing to recap.
func Can(s *session.Session) error {
	switch {
	case s.Live != nil:
		return ErrLive
	case s.Cowork:
		return errors.New("cowork session: no Recaps for read-only accounts")
	case s.Sidechain:
		return errors.New("subagent transcript: no Recap")
	case s.Empty():
		return errors.New("empty session: nothing to recap")
	}
	return nil
}

// stat is os.Stat; a test swaps it to grow the transcript right after the stat.
var stat = os.Stat

// Make writes a Recap for s: it reads the talk, asks model, and parses the reply. It
// records the transcript size the talk was read at, so a later Recap can tell it's
// outdated; only that many bytes are read, even if the file grows meanwhile. Nothing is
// stored here; on error there is no Recap.
func Make(ctx context.Context, s *session.Session, model Model) (Recap, error) {
	if err := Can(s); err != nil {
		return Recap{}, err
	}
	st, err := stat(s.Path)
	if err != nil {
		return Recap{}, err
	}
	talk, err := claude.ReadTalkUpTo(s.Path, st.Size())
	if err != nil {
		return Recap{}, err
	}
	if len(talk) == 0 {
		return Recap{}, errors.New("no talk in the transcript: nothing to recap")
	}
	reply, err := model(ctx, s, Input(s, talk))
	if err != nil {
		return Recap{}, err
	}
	r, err := Parse(reply)
	if err != nil {
		return Recap{}, fmt.Errorf("%w; reply was %q", err, clipKeepingNewlines(reply, 200))
	}
	r.Size, r.At = st.Size(), time.Now().UTC()
	return r, nil
}
