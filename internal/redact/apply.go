package redact

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
