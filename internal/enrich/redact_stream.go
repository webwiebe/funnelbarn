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
		if match, done := w.step(tok); match != "" || done {
			return match != "", nil
		}
	}
}

// WalkSensitiveURLsInJSON reads the first JSON value from r as a stream of
// tokens and calls fn with every URL-shaped string value that RedactURL would
// change, in document order. Object keys are not checked. Memory stays near the
// size of the largest single string. It returns the read error, or
// io.ErrUnexpectedEOF for a truncated document.
func WalkSensitiveURLsInJSON(r io.Reader, fn func(string)) error {
	dec := json.NewDecoder(r)
	dec.UseNumber()
	var w jsonWalk
	for {
		tok, err := dec.Token()
		if err != nil {
			return w.endErr(err)
		}
		match, done := w.step(tok)
		if match != "" {
			fn(match)
		}
		if done {
			return nil
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

// step takes one token. It returns the token when it is a sensitive URL value
// (match, empty otherwise) and reports the end of the top-level value (done).
func (w *jsonWalk) step(tok json.Token) (match string, done bool) {
	d, isDelim := tok.(json.Delim)
	switch {
	case isDelim && (d == '}' || d == ']'):
		w.stack = w.stack[:len(w.stack)-1]
		return "", w.valueDone()
	case w.atKey():
		w.stack[len(w.stack)-1].wantKey = false
		return "", false
	case isDelim:
		w.stack = append(w.stack, jsonFrame{object: d == '{', wantKey: d == '{'})
		return "", false
	}
	if s, ok := tok.(string); ok && looksLikeURL(s) && RedactURL(s) != s {
		match = s
	}
	return match, w.valueDone()
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
