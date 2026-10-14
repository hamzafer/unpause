package claude

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

// Block is one piece of a message's content, as the MCP server hands it out.
type Block struct {
	Type    string `json:"type"` // "text", "tool_use", "tool_result" or "thinking"
	Text    string `json:"text,omitempty"`
	Name    string `json:"name,omitempty"`  // tool_use: the tool's name
	Input   any    `json:"input,omitempty"` // tool_use: the raw JSON input, or a cut string when it is too long
	IsError bool   `json:"is_error,omitempty"`
}

// FullMessage is one user or assistant record with content, in file order.
type FullMessage struct {
	Index  int     `json:"index"`
	Role   string  `json:"role"`
	At     string  `json:"at,omitempty"`
	Blocks []Block `json:"blocks"`
}

// rawBlock is every content block field the full reader uses.
type rawBlock struct {
	Type     string          `json:"type"`
	Text     string          `json:"text"`
	Thinking string          `json:"thinking"`
	Name     string          `json:"name"`
	Input    json.RawMessage `json:"input"`
	Content  json.RawMessage `json:"content"`
	IsError  bool            `json:"is_error"`
}

type record struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Message   *struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// isMessageLine is the cheap pre-filter for user and assistant records.
func isMessageLine(raw []byte) bool {
	return len(raw) > 0 && raw[0] == '{' &&
		(bytes.Contains(raw, []byte(`"type":"user"`)) || bytes.Contains(raw, []byte(`"type":"assistant"`)))
}

// decodeMessage turns one transcript line into its role, timestamp and full blocks.
// ok is false for lines that are not a user or assistant record with content.
// Block text is not cut; callers truncate.
func decodeMessage(raw []byte) (role, at string, blocks []Block, ok bool) {
	var r record
	if json.Unmarshal(raw, &r) != nil || (r.Type != "user" && r.Type != "assistant") || r.Message == nil {
		return "", "", nil, false
	}
	blocks = contentBlocks(r.Message.Content)
	if len(blocks) == 0 {
		return "", "", nil, false
	}
	if t, err := time.Parse(time.RFC3339Nano, r.Timestamp); err == nil {
		at = t.UTC().Format(time.RFC3339)
	}
	return r.Type, at, blocks, true
}

func contentBlocks(content json.RawMessage) []Block {
	if len(content) == 0 {
		return nil
	}
	if content[0] == '"' {
		var s string
		if json.Unmarshal(content, &s) == nil && strings.TrimSpace(s) != "" {
			return []Block{{Type: "text", Text: s}}
		}
		return nil
	}
	var raws []rawBlock
	if json.Unmarshal(content, &raws) != nil {
		return nil
	}
	var out []Block
	for _, b := range raws {
		switch b.Type {
		case "text":
			if strings.TrimSpace(b.Text) != "" {
				out = append(out, Block{Type: "text", Text: b.Text})
			}
		case "thinking":
			if strings.TrimSpace(b.Thinking) != "" {
				out = append(out, Block{Type: "thinking", Text: b.Thinking})
			}
		case "tool_use":
			in := json.RawMessage(bytes.TrimSpace(b.Input))
			if len(in) == 0 {
				in = json.RawMessage("{}")
			}
			out = append(out, Block{Type: "tool_use", Name: b.Name, Input: in})
		case "tool_result":
			out = append(out, Block{Type: "tool_result", Text: flatten(b.Content), IsError: b.IsError})
		}
	}
	return out
}

// flatten reads a tool_result's content: a string, or an array whose text parts are joined by newlines.
func flatten(content json.RawMessage) string {
	if len(content) == 0 {
		return ""
	}
	if content[0] == '"' {
		var s string
		_ = json.Unmarshal(content, &s)
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(content, &parts) != nil {
		return ""
	}
	var texts []string
	for _, p := range parts {
		if p.Type == "text" && p.Text != "" {
			texts = append(texts, p.Text)
		}
	}
	return strings.Join(texts, "\n")
}

// Truncate cuts s to max characters (runes) and says how many were dropped.
func Truncate(s string, max int) string {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	i, n := 0, 0
	for i < len(s) && n < max {
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
		n++
	}
	return fmt.Sprintf("%s[… %d more chars]", s[:i], utf8.RuneCountInString(s[i:]))
}

func cutBlock(b Block, max int) Block {
	b.Text = Truncate(b.Text, max)
	if raw, ok := b.Input.(json.RawMessage); ok && utf8.RuneCount(raw) > max {
		b.Input = Truncate(string(raw), max)
	}
	return b
}

// eachLine streams r line by line, without a line length limit.
func eachLine(r io.Reader, fn func([]byte) error) error {
	br := bufio.NewReaderSize(r, 1<<20)
	for {
		raw, err := br.ReadBytes('\n')
		if len(raw) > 0 {
			if ferr := fn(bytes.TrimSpace(raw)); ferr != nil {
				return ferr
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// ReadMessages streams a transcript and returns the messages with index in [offset, offset+limit),
// plus the total message count. Each block is cut to maxChars characters.
func ReadMessages(path string, offset, limit, maxChars int) ([]FullMessage, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	out := []FullMessage{}
	total := 0
	err = eachLine(f, func(raw []byte) error {
		if !isMessageLine(raw) {
			return nil
		}
		role, at, blocks, ok := decodeMessage(raw)
		if !ok {
			return nil
		}
		i := total
		total++
		if i < offset || i-offset >= limit { // not i >= offset+limit, which overflows
			return nil
		}
		for j := range blocks {
			blocks[j] = cutBlock(blocks[j], maxChars)
		}
		out = append(out, FullMessage{Index: i, Role: role, At: at, Blocks: blocks})
		return nil
	})
	return out, total, err
}
