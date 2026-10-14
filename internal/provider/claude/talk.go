package claude

import (
	"os"
	"strings"
	"time"

	"github.com/hamzafer/unpause/internal/session"
)

// ReadTalk returns a transcript's talk in file order, by Deep search's rules: the text of
// user prompts and assistant replies, with no thinking, tool calls, tool output or text
// Claude Code injected. A record's text blocks are joined into one message.
func ReadTalk(path string) ([]session.Message, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []session.Message
	err = eachLine(f, func(raw []byte) error {
		if !isMessageLine(raw) || isMeta(raw) {
			return nil
		}
		role, at, blocks, ok := decodeMessage(raw)
		if !ok {
			return nil
		}
		var parts []string
		for _, b := range blocks {
			if b.Type != "text" {
				continue
			}
			text := b.Text
			if role == "user" {
				if text, ok = talk(b.Text); !ok {
					continue
				}
			}
			if t := strings.TrimSpace(text); t != "" {
				parts = append(parts, t)
			}
		}
		if len(parts) == 0 {
			return nil
		}
		m := session.Message{Role: role, Text: strings.Join(parts, "\n\n")}
		m.At, _ = time.Parse(time.RFC3339, at)
		out = append(out, m)
		return nil
	})
	return out, err
}
