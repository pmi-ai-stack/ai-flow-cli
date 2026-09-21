package redact

import "io"

// Writer wraps w so every Write call is redacted first — for stderr
// logging and events.jsonl, both of which write one complete line per
// call, starting with the packages that introduce them (phase 3
// onward).
//
// It is not a general-purpose streaming redactor: a secret value split
// across two separate Write calls is not caught. That limitation is
// inherent to redacting a stream rather than a whole buffer; artifacts
// (written whole, via Bytes) are unaffected by it.
func (r *Redactor) Writer(w io.Writer) io.Writer {
	return &redactWriter{r: r, w: w}
}

type redactWriter struct {
	r *Redactor
	w io.Writer
}

// Write redacts p and forwards it to the wrapped writer. It reports the
// length of p, not of the redacted (generally shorter or longer)
// output, since io.Writer callers only ever compare the returned count
// against len(p).
func (rw *redactWriter) Write(p []byte) (int, error) {
	if _, err := rw.w.Write(rw.r.Bytes(p)); err != nil {
		return 0, err
	}
	return len(p), nil
}
