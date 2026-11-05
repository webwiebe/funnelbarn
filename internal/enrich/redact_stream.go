package enrich

import (
	"encoding/json"
	"io"
)

// jsonFrame is one open object or array while a document is read as tokens.
type jsonFrame struct {
	object  bool
	wantKey bool // in an object: the next token is a key (or the closing brace)
}

// HasSensitiveURLInJSON reports whether the first JSON value read from r holds
// a URL-shaped string value that RedactURL would change: the check
// RedactURLsInJSON makes, without building the document in memory. It reads
// the input as a stream of tokens, so memory stays near the size of the
// largest single string, and it stops at the first match. Object keys are not
// checked, as in RedactURLsInJSON.
func HasSensitiveURLInJSON(r io.Reader) (bool, error) {
	dec := json.NewDecoder(r)
	dec.UseNumber()
	var w jsonWalk
	for {
		tok, err := dec.Token()
		if err != nil {
			return false, w.endErr(err)
		}
		if found, done := w.step(tok); found || done {
			return found, nil
		}
	}
}

// jsonWalk tracks where a token stream is inside the document.
type jsonWalk struct {
	stack []jsonFrame
}

// endErr turns a read error into the result: a clean end before any value is
// no match, an end inside a value is a truncated document.
func (w *jsonWalk) endErr(err error) error {
	if err != io.EOF {
		return err
	}
	if len(w.stack) == 0 {
		return nil
	}
	return io.ErrUnexpectedEOF
}

// step takes one token. It reports a sensitive URL value (found) or the end of
// the top-level value (done).
func (w *jsonWalk) step(tok json.Token) (found, done bool) {
	d, isDelim := tok.(json.Delim)
	switch {
	case isDelim && (d == '}' || d == ']'):
		w.stack = w.stack[:len(w.stack)-1]
		return false, w.valueDone()
	case w.atKey():
		w.stack[len(w.stack)-1].wantKey = false
		return false, false
	case isDelim:
		w.stack = append(w.stack, jsonFrame{object: d == '{', wantKey: d == '{'})
		return false, false
	}
	if s, ok := tok.(string); ok && looksLikeURL(s) && RedactURL(s) != s {
		return true, false
	}
	return false, w.valueDone()
}

// atKey reports whether the next token is an object key.
func (w *jsonWalk) atKey() bool {
	n := len(w.stack)
	return n > 0 && w.stack[n-1].wantKey
}

// valueDone records that a value ended in the innermost frame and reports
// whether that completed the top-level value.
func (w *jsonWalk) valueDone() bool {
	if len(w.stack) == 0 {
		return true
	}
	if top := &w.stack[len(w.stack)-1]; top.object {
		top.wantKey = true
	}
	return false
}
