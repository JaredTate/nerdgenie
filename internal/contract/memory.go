package contract

import (
	"context"
	"time"
)

// MemoryHintLines is how many lines of memory ride along at the end of every
// prompt. Three is enough to be useful and small enough to be free.
const MemoryHintLines = 3

// Fact is one thing the agent knows, with where it learned it and when.
// Nothing is ever deleted: a new fact supersedes an old one, and the old one
// stays searchable.
type Fact struct {
	// ID identifies the fact.
	ID string
	// Text is the fact itself, in plain words.
	Text string
	// Source says where it came from, such as a file, a website, or the user.
	Source string
	// Recorded is when it was written down.
	Recorded time.Time
	// Supersedes is the id of the fact this one replaces, or empty.
	Supersedes string
}

// Memory is what the agent knows across tasks: two files with hard size limits,
// a folder for anything larger, and a full-text index over all of it plus every
// past message.
type Memory interface {
	// Search finds facts matching a query, newest first, up to a limit.
	Search(ctx context.Context, query string, limit int) ([]Fact, error)
	// Get returns one fact by its id.
	Get(ctx context.Context, id string) (Fact, error)
	// Save writes a batch of facts in one atomic step.
	Save(ctx context.Context, facts []Fact) error
	// Hint returns up to MemoryHintLines lines for the end of the prompt, and
	// returns nothing when nothing matches.
	Hint(ctx context.Context, query string) ([]string, error)
}
