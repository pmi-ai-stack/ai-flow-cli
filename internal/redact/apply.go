package redact

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
)

// String replaces every discovered secret's value in s with its
// HashPrefix token. Longest values are substituted first so that one
// secret being a substring of another cannot leave a partial value
// exposed.
func (r *Redactor) String(s string) string {
	values := make([]string, len(r.secrets))
	for i, sec := range r.secrets {
		values[i] = sec.Value
	}
	return replaceLongestFirst(s, values)
}

// Bytes is String for the byte-slice shape artifacts and captured tool
// output arrive in.
func (r *Redactor) Bytes(b []byte) []byte {
	return []byte(r.String(string(b)))
}

// Output redacts rendered command output — the JSON envelope or its
// --human rendering. Besides each secret's raw value it replaces the
// secret's JSON-string-escaped form with the same token: JSON escaping
// is per character, so a secret containing `"` or `\` appears inside
// any encoded string exactly as its escaped form, which String alone
// would miss. Working on the rendered bytes rather than on decoded
// values keeps the envelope's field order untouched. Longest first, as
// everywhere in this package.
func (r *Redactor) Output(b []byte) []byte {
	type pair struct{ from, to string }
	var pairs []pair
	for _, sec := range r.secrets {
		token := HashPrefix(sec.Value)
		pairs = append(pairs, pair{sec.Value, token})
		if esc := jsonEscaped(sec.Value); esc != sec.Value {
			pairs = append(pairs, pair{esc, token})
		}
	}
	slices.SortStableFunc(pairs, func(a, b pair) int { return len(b.from) - len(a.from) })
	s := string(b)
	for _, p := range pairs {
		s = strings.ReplaceAll(s, p.from, p.to)
	}
	return []byte(s)
}

// jsonEscaped is v as it appears between the quotes of a JSON string
// encoded without HTML escaping — the envelope's own encoding.
func jsonEscaped(v string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return v
	}
	out := strings.TrimSuffix(buf.String(), "\n")
	return out[1 : len(out)-1]
}
