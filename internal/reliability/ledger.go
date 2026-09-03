// The delivery ledger follows the one in Hermes at
// ~/Code/hermes-agent/gateway/delivery_ledger.py. A reply that was written by
// the model but not yet confirmed as delivered is the one thing an agent can
// lose without a trace: the turn has already been paid for, and the text exists
// only in memory. Hermes writes a row before the send and marks it after, and
// the two lessons kept here are that a reply sent again after a crash must say
// it may be a duplicate, because honest at-least-once beats a silent double
// message, and that attempts are capped and old replies expire, so that a reply
// nobody can deliver cannot spin forever.

package reliability

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// DuplicateMarker goes in front of a reply that is sent again after a crash,
// because the first send may have arrived and the user deserves to be told
// which of the two this is.
const DuplicateMarker = "(You may have this already. Coeus restarted while it was sending it.)\n\n"

const (
	// MaxDeliveryAttempts is how many times one reply is sent again before it is
	// given up on, so that a reply nobody can deliver cannot spin forever.
	MaxDeliveryAttempts = 3
	// DeliveryLifetime is how long a reply is worth sending again. After a day
	// the answer is stale and a message out of nowhere confuses more than it
	// helps.
	DeliveryLifetime = 24 * time.Hour
	// MaxUndeliveredReplies caps how many undelivered replies are held in memory
	// while the log is read, because every buffer in Coeus has a cap. A log with
	// more than this waiting is a log to read by hand.
	MaxUndeliveredReplies = 1000
)

// ReplyState says where one reply got to.
type ReplyState string

const (
	// ReplyPending means the reply was written down and has not been confirmed
	// as delivered.
	ReplyPending ReplyState = "pending"
	// ReplyDelivered means the channel took it.
	ReplyDelivered ReplyState = "delivered"
	// ReplyGivenUp means it was tried enough times, or is old enough, that
	// sending it again would do more harm than good.
	ReplyGivenUp ReplyState = "given up"
)

// ReplyBody is the body of a reply event in the log. The first event of a reply
// carries the text and no identifier, because its own sequence number is the
// identifier; every event after it names that identifier and says what happened
// next.
type ReplyBody struct {
	// ID is the reply this event is about, and is empty on the first one.
	ID string `json:"id,omitempty"`
	// Channel is the channel the reply goes out on.
	Channel string `json:"channel,omitempty"`
	// Text is what the user is told, and is only on the first event.
	Text string `json:"text,omitempty"`
	// State says where the reply got to.
	State ReplyState `json:"state"`
	// Attempts is how many times it has been sent again.
	Attempts int `json:"attempts"`
	// WrittenAt is when the reply was first written down, which is what the day
	// it is worth sending for is counted from.
	WrittenAt time.Time `json:"writtenAt"`
}

// Reply is one reply the ledger is holding on to.
type Reply struct {
	// ID is what the ledger knows it by.
	ID string
	// TaskID is the task it belongs to.
	TaskID string
	// Channel is where it goes.
	Channel string
	// Text is what it says.
	Text string
	// Attempts is how many times it has been sent again.
	Attempts int
	// WrittenAt is when it was first written down.
	WrittenAt time.Time
}

// Ledger writes every reply into the event log before it is sent and marks it
// delivered after, so that a crash between the two is a reply that is sent
// again rather than a reply that is lost.
type Ledger struct {
	store contract.Store
	clock contract.Clock
}

// NewLedger returns the ledger over one event log.
func NewLedger(store contract.Store, clock contract.Clock) *Ledger {
	return &Ledger{store: store, clock: clock}
}

// Record writes a reply into the log before anything tries to send it, and
// returns it with the identifier the ledger knows it by.
func (ledger *Ledger) Record(ctx context.Context, taskID string, channel string, text string) (Reply, error) {
	if text == "" {
		return Reply{}, errors.New("a reply with no text was written to the ledger, so give the ledger the words the user will read")
	}
	now := ledger.clock.Now()
	sequence, err := ledger.append(ctx, taskID, ReplyBody{
		Channel:   channel,
		Text:      text,
		State:     ReplyPending,
		WrittenAt: now,
	})
	if err != nil {
		return Reply{}, err
	}
	return Reply{
		ID:        strconv.FormatInt(sequence, 10),
		TaskID:    taskID,
		Channel:   channel,
		Text:      text,
		WrittenAt: now,
	}, nil
}

// MarkDelivered writes down that the reply reached the user, which is what
// stops it being sent again after a restart.
func (ledger *Ledger) MarkDelivered(ctx context.Context, reply Reply) error {
	_, err := ledger.append(ctx, reply.TaskID, ReplyBody{
		ID:        reply.ID,
		State:     ReplyDelivered,
		Attempts:  reply.Attempts,
		WrittenAt: reply.WrittenAt,
	})
	return err
}

// Undelivered lists the replies that were written down and never confirmed as
// delivered, oldest first. It reads the whole log, because that is what
// rebuilding state after a crash means.
func (ledger *Ledger) Undelivered(ctx context.Context) ([]Reply, error) {
	waiting := map[string]Reply{}
	order := []string{}

	err := ledger.store.Replay(ctx, func(event contract.Event) error {
		body := ReplyBody{}
		if event.Kind != contract.EventReply || json.Unmarshal(event.Body, &body) != nil {
			return nil
		}
		id := body.ID
		if id == "" {
			id = strconv.FormatInt(event.Sequence, 10)
		}
		if body.State != ReplyPending {
			delete(waiting, id)
			return nil
		}
		if _, known := waiting[id]; !known {
			if len(waiting) >= MaxUndeliveredReplies {
				return fmt.Errorf("more than %d replies are waiting to be sent, which is more than Coeus holds at once: read the log to see what happened", MaxUndeliveredReplies)
			}
			order = append(order, id)
		}
		waiting[id] = replyFrom(id, event, body, waiting[id])
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("the replies waiting to be sent could not be read out of the log: %w", err)
	}

	found := make([]Reply, 0, len(waiting))
	for _, id := range order {
		if reply, still := waiting[id]; still {
			found = append(found, reply)
		}
	}
	return found, nil
}

// Resend sends every undelivered reply again with the duplicate marker in front
// of it, and returns how many arrived. A reply that has been tried enough times,
// or that was written more than a day ago, is given up on with a line in the log
// saying so.
func (ledger *Ledger) Resend(ctx context.Context, send func(ctx context.Context, channel string, text string) error) (int, error) {
	if send == nil {
		return 0, errors.New("the ledger was asked to send replies again with no way of sending them, so pass the function that delivers one")
	}
	waiting, err := ledger.Undelivered(ctx)
	if err != nil {
		return 0, err
	}

	arrived := 0
	for _, reply := range waiting {
		giveUp, err := ledger.giveUpOn(ctx, reply)
		if err != nil {
			return arrived, err
		}
		if giveUp {
			continue
		}
		reply.Attempts++
		if _, err := ledger.append(ctx, reply.TaskID, bodyOf(reply, ReplyPending)); err != nil {
			return arrived, err
		}
		if err := send(ctx, reply.Channel, DuplicateMarker+reply.Text); err != nil {
			continue
		}
		if err := ledger.MarkDelivered(ctx, reply); err != nil {
			return arrived, err
		}
		arrived++
	}
	return arrived, nil
}

// giveUpOn says whether the reply has had its chances, and writes the line in
// the log that says so.
func (ledger *Ledger) giveUpOn(ctx context.Context, reply Reply) (bool, error) {
	tooOld := ledger.clock.Now().Sub(reply.WrittenAt) > DeliveryLifetime
	if reply.Attempts < MaxDeliveryAttempts && !tooOld {
		return false, nil
	}
	if _, err := ledger.append(ctx, reply.TaskID, bodyOf(reply, ReplyGivenUp)); err != nil {
		return true, err
	}
	return true, nil
}

// append writes one reply event and returns the number the log gave it.
func (ledger *Ledger) append(ctx context.Context, taskID string, body ReplyBody) (int64, error) {
	written, err := json.Marshal(body)
	if err != nil {
		return 0, fmt.Errorf("the reply could not be turned into JSON to write it down: %w", err)
	}
	sequence, err := ledger.store.Append(ctx, contract.Event{
		Occurred: ledger.clock.Now(),
		TaskID:   taskID,
		Kind:     contract.EventReply,
		Body:     written,
	})
	if err != nil {
		return 0, fmt.Errorf("the reply could not be written to the log, and nothing is sent that was not written down first: %w", err)
	}
	return sequence, nil
}

// bodyOf turns a reply back into the body of an event about it.
func bodyOf(reply Reply, state ReplyState) ReplyBody {
	return ReplyBody{
		ID:        reply.ID,
		Channel:   reply.Channel,
		State:     state,
		Attempts:  reply.Attempts,
		WrittenAt: reply.WrittenAt,
	}
}

// replyFrom builds what is known about a reply from one event about it and
// whatever an earlier event said, because the first event carries the text and
// the ones after it carry the attempts.
func replyFrom(id string, event contract.Event, body ReplyBody, known Reply) Reply {
	reply := Reply{
		ID:        id,
		TaskID:    event.TaskID,
		Channel:   body.Channel,
		Text:      body.Text,
		Attempts:  body.Attempts,
		WrittenAt: body.WrittenAt,
	}
	if reply.Text == "" {
		reply.Text = known.Text
	}
	if reply.Channel == "" {
		reply.Channel = known.Channel
	}
	if reply.WrittenAt.IsZero() {
		reply.WrittenAt = known.WrittenAt
	}
	return reply
}
