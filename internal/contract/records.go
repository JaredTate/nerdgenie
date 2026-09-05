package contract

// Records is the record of the task running now, as the tools that read it see
// it. The keeper in internal/record is one, and so is what the turn loop hands
// the tools before the first tool call has made a record. The job tool takes
// the new job's ask from it word for word, and the task tool writes through the
// same keeper with an Apply of its own beside this, because the update it
// writes is the record package's type and this package imports nothing outside
// the standard library.
type Records interface {
	// Record returns a copy of the record as it stands, which is empty until
	// the first tool call has made one.
	Record() Record
}
