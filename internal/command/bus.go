package command

import (
	"context"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// Command is one unit of bookkeeping work. Commands are plain data so they can
// cross a process boundary (see Envelope); the consumer binds them to its own
// write connection through Deps.
type Command interface {
	// Kind labels the command in metrics and spans and selects its payload type.
	Kind() string
	// Project is the project the command belongs to, or "" when it names none.
	Project() string
	// Apply performs the work. ctx carries a span and no request cancellation.
	Apply(ctx context.Context, d Deps) error
}

// Store is the write-side storage a consumer applies commands to.
// *repository.Store implements it.
type Store interface {
	RecordEvaluation(ctx context.Context, eval repository.FlagEvaluation) error
	TouchAPIKey(ctx context.Context, keySHA256 string) error
	FlagByKey(ctx context.Context, projectID, flagKey string) (repository.FeatureFlag, error)
	TouchFlagEvaluated(ctx context.Context, flagID string) error
	EnsureAutoFlag(ctx context.Context, f repository.FeatureFlag) (repository.FeatureFlag, error)
	CountAutoFlags(ctx context.Context, projectID string) (int, error)
}

// Deps is what a consumer gives commands to apply against.
type Deps struct {
	Store Store
	// MarkFlagsEvaluated is the project health service's marker, which caches
	// the first success so later marks skip the write.
	MarkFlagsEvaluated func(ctx context.Context, projectID string) error
}

// Bus carries commands from request handlers to the single writer. The
// in-process Dispatcher and the Redis-backed RedisBus implement it.
type Bus interface {
	// Start launches the consumer, if this bus has one.
	Start(ctx context.Context)
	// Submit hands c to the bus and returns how long the hand-off waited. It
	// never drops a command.
	Submit(ctx context.Context, c Command) time.Duration
	// Flush blocks until every command submitted before the call is applied.
	Flush(ctx context.Context) error
	// Close stops accepting commands and drains what is queued, within ctx.
	Close(ctx context.Context) error
}

// Queue is the durable transport RedisBus needs: a reliable list with
// at-least-once delivery. internal/queue implements it on Redis/Valkey.
type Queue interface {
	// Publish appends one encoded command.
	Publish(ctx context.Context, payload []byte) error
	// Receive blocks for up to the queue's poll timeout and returns the next
	// payload with an ack that removes it from the processing list. It returns
	// a nil payload and nil error when the timeout passes with nothing queued.
	Receive(ctx context.Context) (payload []byte, ack func(context.Context) error, err error)
	// Recover moves anything left in the processing list (a consumer that died
	// between Receive and ack) back onto the queue and returns how many.
	Recover(ctx context.Context) (int, error)
	// Len returns the number of payloads waiting, processing list included.
	Len(ctx context.Context) (int64, error)
}

var (
	_ Bus   = (*Dispatcher)(nil)
	_ Store = (*repository.Store)(nil)
)
