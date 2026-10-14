package recap

import (
	"context"
	"sort"

	"github.com/hamzafer/unpause/internal/session"
)

// Todo picks the Sessions backfill should recap: those with no Recap or an outdated one,
// newest first, at most limit of them (0 means all). Every Session Can refuses (Live,
// Cowork, empty, sidechain) is skipped.
func Todo(sessions []*session.Session, st Store, limit int) []*session.Session {
	var out []*session.Session
	for _, s := range sessions {
		if Can(s) != nil {
			continue
		}
		if r, ok := st[s.ID]; ok && !Outdated(r, s) {
			continue
		}
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].LastActive.After(out[j].LastActive) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// Count reports how many of sessions have a Recap, and how many of those are outdated.
func Count(sessions []*session.Session, st Store) (have, outdated int) {
	for _, s := range sessions {
		r, ok := st[s.ID]
		if !ok {
			continue
		}
		have++
		if Outdated(r, s) {
			outdated++
		}
	}
	return have, outdated
}

// Result is the outcome for one Session in a backfill.
type Result struct {
	Session *session.Session
	Recap   Recap
	Err     error
}

// Backfill makes a Recap for each Session in turn and stores it with put. report is
// called once per Session, in order. An Account whose login failed fails its remaining
// Sessions at once, because the model (autoname.Ask) remembers the failure.
func Backfill(ctx context.Context, sessions []*session.Session, model Model, put func(id string, r Recap) error, report func(Result)) {
	for _, s := range sessions {
		if ctx.Err() != nil {
			return
		}
		res := Result{Session: s}
		res.Recap, res.Err = Make(ctx, s, model)
		if res.Err == nil {
			res.Err = put(s.ID, res.Recap)
		}
		report(res)
	}
}
