package contract

import (
	"context"
	"encoding/json"
	"time"
)

// EventKind says what one line of the event log records.
type EventKind string

const (
	// EventMessage is a message from a user or a reply to one.
	EventMessage EventKind = "message"
	// EventToolCall is the model asking for a tool.
	EventToolCall EventKind = "tool call"
	// EventToolResult is the full text a tool returned, which is what "read r7"
	// brings back after the result has left the working context.
	EventToolResult EventKind = "tool result"
	// EventPermissionDecision is one ruling of the permission function.
	EventPermissionDecision EventKind = "permission decision"
	// EventFileChange is a file the agent wrote, with its prior contents, which
	// is what "/undo" restores from.
	EventFileChange EventKind = "file change"
	// EventRecordChange is one change to a task or job record.
	EventRecordChange EventKind = "record change"
	// EventCheckpoint is a numbered saved copy of a record, which is what
	// "/tasks 17 back 3" reloads.
	EventCheckpoint EventKind = "checkpoint"
	// EventReply is a reply written down before it was sent, so that a crash
	// between writing and sending cannot lose it.
	EventReply EventKind = "reply"
)

// KnownEventKind says whether the kind is one the log recognises.
func KnownEventKind(kind EventKind) bool {
	switch kind {
	case EventMessage, EventToolCall, EventToolResult, EventPermissionDecision,
		EventFileChange, EventRecordChange, EventCheckpoint, EventReply:
		return true
	default:
		return false
	}
}

// Event is one line of the append-only log. Nothing in the log is ever changed
// or removed, because the log is what decides what really happened.
type Event struct {
	// Sequence is the log's own number for this event, and it only ever grows.
	Sequence int64
	// Occurred is when it happened.
	Occurred time.Time
	// TaskID is the task or job it belongs to, or empty for events that belong
	// to no task.
	TaskID string
	// Kind says what it is.
	Kind EventKind
	// Body is the event's own fields, whose shape depends on the kind.
	Body json.RawMessage
}

// FileChangeBody is the body of a file-change event: which file the agent
// wrote and what it held before, which is what "/undo" puts back. The write and
// edit tools record one before touching a file, and the undo command reads them
// back newest first.
type FileChangeBody struct {
	// Path is the file that was written.
	Path string `json:"path"`
	// Existed says the file was there before the change. When it was not, undo
	// removes it rather than restoring it.
	Existed bool `json:"existed"`
	// PriorContents is everything the file held before the change, and is empty
	// when the file did not exist.
	PriorContents []byte `json:"priorContents,omitempty"`
	// Mode is the file's permission bits before the change.
	Mode uint32 `json:"mode,omitempty"`
}

// EventRange is a span of sequence numbers, from From up to and including To.
type EventRange struct {
	// From is the first sequence number wanted.
	From int64
	// To is the last one wanted.
	To int64
}

// Store is the event log: one append-only table in the single SQLite file, with
// one writer and the four ways of reading it back that the harness needs.
type Store interface {
	// Append writes one event and returns the sequence number it was given.
	Append(ctx context.Context, event Event) (int64, error)
	// ByTask returns every event of one task or job, in order.
	ByTask(ctx context.Context, taskID string) ([]Event, error)
	// ByKind returns every event of one kind, in order.
	ByKind(ctx context.Context, kind EventKind) ([]Event, error)
	// ByID returns one event by its sequence number.
	ByID(ctx context.Context, sequence int64) (Event, error)
	// ByRange returns every event in a span of sequence numbers.
	ByRange(ctx context.Context, span EventRange) ([]Event, error)
	// Replay hands every event to a function in order, which is how the agent
	// rebuilds its state after a crash. It stops at the first error.
	Replay(ctx context.Context, hand func(event Event) error) error
}
