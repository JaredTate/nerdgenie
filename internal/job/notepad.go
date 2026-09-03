// The notepad a job carries from one of its tasks to the next is Hermes'
// design, at ~/Code/hermes-agent/cron/notepad.py, where a scheduled job keeps a
// small scratch pad of its own so that the next run knows where the last one got
// to, and an empty notepad renders as nothing at all so that a job which never
// uses it sends a prompt the provider can still reuse from its cache. One
// incident per distinct error, reported once, is Hermes' design too, at
// ~/Code/hermes-agent/cron/incidents.py. The Go here is written fresh.

package job

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// signatureLength is how much of the hash of an error is kept as its signature.
// Six bytes is short enough to read out loud and long enough that two different
// errors will not collide in the fifty a job remembers.
const signatureLength = 12

// errorTextRead is how much of an error's text decides its signature. Two
// failures that begin the same way are the same trouble, however long the tail
// each one printed.
const errorTextRead = 200

// Incident is one distinct thing that has gone wrong with a job. Two failures
// whose text reads the same are one incident counted twice, so that a job
// failing every hour for a week tells the user once rather than a hundred and
// sixty-eight times.
type Incident struct {
	// Signature is the short hash of the error's text, which is what makes two
	// failures the same incident.
	Signature string `json:"signature"`
	// Error is the text of the failure, as it was first seen.
	Error string `json:"error"`
	// FirstSeen is when it happened the first time.
	FirstSeen time.Time `json:"firstSeen"`
	// LastSeen is when it last happened.
	LastSeen time.Time `json:"lastSeen"`
	// Count is how many times it has happened.
	Count int `json:"count"`
}

// Incidents returns the distinct failures one job has met, oldest first, which
// is what "/cron 3" shows under the job.
func (jobs *Jobs) Incidents(_ context.Context, jobID string) ([]Incident, error) {
	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	held, err := jobs.find(jobID)
	if err != nil {
		return nil, err
	}
	return append([]Incident(nil), held.state.Incidents...), nil
}

// Notepad returns what a job's tasks have written down for the ones after them.
func (jobs *Jobs) Notepad(_ context.Context, jobID string) (string, error) {
	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	held, err := jobs.find(jobID)
	if err != nil {
		return "", err
	}
	return held.state.Notepad, nil
}

// AppendNote writes one line onto a job's notepad, for the tasks that come
// after. The notepad is capped, and the oldest lines are dropped to make room,
// because a job that runs every hour for a year must not fill the disk.
func (jobs *Jobs) AppendNote(ctx context.Context, jobID string, note string) error {
	trimmed := strings.TrimSpace(note)
	if trimmed == "" {
		return fmt.Errorf("a note for job %s says nothing, so write the line the next task should read", jobID)
	}
	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	held, err := jobs.find(jobID)
	if err != nil {
		return err
	}
	if err := jobs.appendState(ctx, jobID, stateEvent{Marker: stateMarker, Note: trimmed}); err != nil {
		return err
	}
	held.state.Notepad = appendToNotepad(held.state.Notepad, trimmed)
	return nil
}

// appendToNotepad puts one line on the end of a notepad and drops whole lines
// off the front until what is left is inside the cap.
func appendToNotepad(notepad string, note string) string {
	grown := notepad + note + "\n"
	for len(grown) > NotepadBytes {
		_, rest, split := strings.Cut(grown, "\n")
		if !split || rest == "" {
			// One line on its own is longer than the whole notepad, so its ending
			// is kept: a note says what it found at the end of itself.
			return theLastBytesWholeLettersFit(grown, NotepadBytes)
		}
		grown = rest
	}
	return grown
}

// theLastBytesWholeLettersFit is the end of a piece of text, inside the cap and
// beginning at a whole letter, because a note cut halfway through an accented
// letter is a note that is no longer text.
func theLastBytesWholeLettersFit(text string, keep int) string {
	if len(text) <= keep {
		return text
	}
	for at := len(text) - keep; at < len(text); at++ {
		if utf8.RuneStart(text[at]) {
			return text[at:]
		}
	}
	return ""
}

// noteIncident counts one failure against the job's incidents and says whether
// this is a kind of failure the job has not met before, which is the one worth
// telling the user about.
func noteIncident(incidents []Incident, errorText string, now time.Time) ([]Incident, bool) {
	signature := signatureOf(errorText)
	for at := range incidents {
		if incidents[at].Signature != signature {
			continue
		}
		counted := append([]Incident(nil), incidents...)
		counted[at].Count++
		counted[at].LastSeen = now
		return counted, false
	}
	added := append([]Incident(nil), incidents...)
	added = append(added, Incident{
		Signature: signature, Error: oneLine(errorText), FirstSeen: now, LastSeen: now, Count: 1,
	})
	if len(added) > MaxIncidents {
		added = added[len(added)-MaxIncidents:]
	}
	return added, true
}

// signatureOf is what makes two failures the same incident: the short hash of the
// beginning of the error's text, with the capitals and the spacing taken out, so
// that the same trouble reported twice in slightly different words is counted
// once.
func signatureOf(errorText string) string {
	folded := strings.ToLower(strings.Join(strings.Fields(errorText), " "))
	if len(folded) > errorTextRead {
		folded = folded[:errorTextRead]
	}
	sum := sha256.Sum256([]byte(folded))
	return hex.EncodeToString(sum[:])[:signatureLength]
}

// oneLine folds a piece of text onto one line, because an incident is one line
// in a listing.
func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
