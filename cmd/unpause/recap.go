package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/hamzafer/unpause/internal/config"
	"github.com/hamzafer/unpause/internal/recap"
	"github.com/hamzafer/unpause/internal/session"
)

// recapBackfill is `unpause recap` without --session: it recaps the Sessions that have
// no Recap or an outdated one, newest first, one line per Session.
func recapBackfill(ctx context.Context, sessions []*session.Session, model recap.Model, limit int, dryRun bool) error {
	path := recap.Path(config.CachePath())
	if err := recap.Migrate(path, sessions); err != nil {
		return err
	}
	st, err := recap.Load(path)
	if err != nil {
		return err
	}
	todo := recap.Todo(sessions, st, limit)
	if len(todo) == 0 {
		fmt.Println("every session already has a current Recap")
		return nil
	}
	line := func(s *session.Session, what string) {
		fmt.Printf("%s %-9s %-24s %s\n", s.ShortID(), s.Account, session.Clip(s.Repo(), 24), what)
	}
	if dryRun {
		fmt.Printf("dry run: %d sessions would get a Recap, nothing is sent to Haiku\n", len(todo))
		for _, s := range todo {
			why := "none yet"
			if _, ok := st.Get(s); ok {
				why = "outdated"
			}
			line(s, fmt.Sprintf("%-8s %s", why, session.Clip(s.Title(), 50)))
		}
		return nil
	}
	done, failed := 0, 0
	recap.Backfill(ctx, todo, model, func(key string, r recap.Recap) error { return recap.Put(path, key, r) }, func(r recap.Result) {
		if r.Err != nil {
			failed++
			line(r.Session, "✗ "+r.Err.Error())
			return
		}
		done++
		line(r.Session, "→ "+session.Clip(r.Recap.Goal, 60))
	})
	fmt.Printf("recapped %d, failed %d\n", done, failed)
	if failed > 0 && done == 0 {
		return fmt.Errorf("every recap call failed")
	}
	return nil
}

// recapStatus is doctor's Recap line: off, or on with how many Sessions have one.
func recapStatus(cfg *config.Config) string {
	if !cfg.Recap.Enabled {
		return "off ([recap] enabled = true in the config turns them on)"
	}
	sessions, err := loadSessions(cfg, true)
	if err != nil {
		return "on (sessions: " + err.Error() + ")"
	}
	st, err := recap.Load(recap.Path(config.CachePath()))
	if err != nil {
		return "on (" + err.Error() + ")"
	}
	var can []*session.Session
	for _, s := range sessions {
		// A Live Session gets its Recap when it ends, so it still counts.
		if err := recap.Can(s); err == nil || errors.Is(err, recap.ErrLive) {
			can = append(can, s)
		}
	}
	have, outdated := recap.Count(can, st)
	out := fmt.Sprintf("on, %d of %d sessions have one", have, len(can))
	if outdated > 0 {
		out += fmt.Sprintf(" (%d outdated)", outdated)
	}
	return out
}
