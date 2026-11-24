package faults

import (
	"bytes"
	"encoding/json"
	"math"
	"unicode/utf8"

	"github.com/sadayamuthu/switchyard"

	"github.com/sadayamuthu/agentchaos/internal/experiment"
)

// mutate returns tr's result corrupted as a partial or malformed fault f
// describes.
func mutate(f experiment.Fault, tr *switchyard.ToolResult) json.RawMessage {
	if f.Type == "partial" {
		return mapText(tr, func(s string) string {
			n := f.Bytes
			if n <= 0 {
				n = int(math.Floor(float64(len(s)) * f.Fraction))
			}
			return cut(s, n)
		})
	}
	switch f.Corrupt {
	case "drop-fields":
		return withContent(tr, nil)
	case "wrong-types":
		return withContent(tr, json.RawMessage(`"content"`))
	default: // invalid-json
		return mapText(tr, func(s string) string { return cut(s, len(s)/2) + `{"` })
	}
}

// cut returns at most the first n bytes of s, shortened further if needed so
// that it ends on a UTF-8 boundary.
func cut(s string, n int) string {
	if n >= len(s) {
		return s
	}
	if n < 0 {
		n = 0
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// mapText returns tr with fn applied to the text of each text content item.
// Other items, and items that cannot be decoded, are kept as they are.
func mapText(tr *switchyard.ToolResult, fn func(string) string) json.RawMessage {
	out := *tr
	out.Rest = withoutStructured(tr.Rest)
	out.Content = make([]json.RawMessage, len(tr.Content))
	for i, item := range tr.Content {
		out.Content[i] = item
		var m map[string]json.RawMessage
		if json.Unmarshal(item, &m) != nil {
			continue
		}
		var typ, text string
		if json.Unmarshal(m["type"], &typ) != nil || typ != "text" || json.Unmarshal(m["text"], &text) != nil {
			continue
		}
		m["text"] = marshal(fn(text))
		out.Content[i] = marshal(m)
	}
	return out.Marshal()
}

// withoutStructured returns a copy of rest without structuredContent. A
// server built with typed tools repeats its data there, so leaving it would
// hand the agent the complete, valid result despite the fault.
func withoutStructured(rest map[string]json.RawMessage) map[string]json.RawMessage {
	m := make(map[string]json.RawMessage, len(rest))
	for k, v := range rest {
		if k != "structuredContent" {
			m[k] = v
		}
	}
	return m
}

// withContent returns tr's result with its content member replaced by
// content, or removed if content is nil. isError and every other member are
// kept.
func withContent(tr *switchyard.ToolResult, content json.RawMessage) json.RawMessage {
	m := make(map[string]json.RawMessage, len(tr.Rest)+2)
	for k, v := range tr.Rest {
		m[k] = v
	}
	delete(m, "content")
	delete(m, "isError")
	delete(m, "structuredContent")
	if tr.IsError {
		m["isError"] = json.RawMessage("true")
	}
	if content != nil {
		m["content"] = content
	}
	return marshal(m)
}

// marshal encodes v as JSON without HTML escaping.
func marshal(v any) json.RawMessage {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v) // strings and maps of raw JSON always encode
	return bytes.TrimRight(b.Bytes(), "\n")
}
