package audit

import "context"

// Sink consumes audit events. Implementations are responsible for assigning
// chain-level fields (SchemaVersion, Seq, Timestamp if zero, Prev, Hash) on
// each Write so callers cannot accidentally break the chain.
type Sink interface {
	Write(ctx context.Context, e Event) error
	Close() error
}
