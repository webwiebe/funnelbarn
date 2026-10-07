package command

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// Envelope is the wire form of a command on the queue, after BugBarn's
// QueueItem.
type Envelope struct {
	Kind       string          `json:"kind"`
	ProjectID  string          `json:"project_id,omitempty"`
	ReceivedAt time.Time       `json:"received_at"`
	Payload    json.RawMessage `json:"payload"`
}

// Encode serialises c into an Envelope. A RecordEvaluation without an ID gets
// one here, so every redelivery of the encoded bytes inserts the same row.
func Encode(c Command, receivedAt time.Time) ([]byte, error) {
	if re, ok := c.(RecordEvaluation); ok && re.Eval.ID == "" {
		id, err := newID()
		if err != nil {
			return nil, err
		}
		re.Eval.ID = id
		c = re
	}
	payload, err := json.Marshal(c)
	if err != nil {
		return nil, fmt.Errorf("encode %s: %w", c.Kind(), err)
	}
	return json.Marshal(Envelope{
		Kind:       c.Kind(),
		ProjectID:  c.Project(),
		ReceivedAt: receivedAt.UTC(),
		Payload:    payload,
	})
}

// Decode parses an Envelope and returns its command.
func Decode(b []byte) (Command, Envelope, error) {
	var env Envelope
	if err := json.Unmarshal(b, &env); err != nil {
		return nil, env, fmt.Errorf("decode envelope: %w", err)
	}
	var (
		c   Command
		err error
	)
	switch env.Kind {
	case KindRecordEvaluation:
		c, err = decodeAs[RecordEvaluation](env.Payload)
	case KindTouchAPIKey:
		c, err = decodeAs[TouchAPIKey](env.Payload)
	case KindTouchFlagEvaluated:
		c, err = decodeAs[TouchFlagEvaluated](env.Payload)
	case KindMarkFlagsEvaluated:
		c, err = decodeAs[MarkFlagsEvaluated](env.Payload)
	case KindEnsureAutoFlag:
		c, err = decodeAs[EnsureAutoFlag](env.Payload)
	default:
		return nil, env, fmt.Errorf("decode envelope: unknown kind %q", env.Kind)
	}
	if err != nil {
		return nil, env, fmt.Errorf("decode %s: %w", env.Kind, err)
	}
	return c, env, nil
}

func decodeAs[T Command](payload json.RawMessage) (Command, error) {
	var v T
	if err := json.Unmarshal(payload, &v); err != nil {
		return nil, err
	}
	return v, nil
}

func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:], nil
}
