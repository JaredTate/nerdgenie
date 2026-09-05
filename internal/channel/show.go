package channel

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// DefaultShowDeadline is how long a show may take to answer when the socket has
// no answer deadline of its own, which is what the shipped caps give it. A show
// reads the log and prints a record, and ten seconds is far past what that
// takes on a log that is well.
const DefaultShowDeadline = 10 * time.Second

// MaxShowsInFlight is how many shows one screen may have unanswered at once. A
// person opens one result at a time, and a screen with more of them out than
// this has lost track of its own asks.
const MaxShowsInFlight = 4

// show hands a screen's show to the program's hook on a goroutine of its own,
// so that a slow read of the log cannot hold up the next thing the screen
// sends, and writes the answer to that screen alone, because a result one
// person opened is not a reply to everyone. It refuses a show when nothing is
// wired to answer one and when the screen already has as many out as the cap
// allows, and either refusal keeps the screen connected.
func (socket *Socket) show(attached *client, envelope contract.SocketEnvelope) error {
	if socket.options.Show == nil {
		return attached.write(errorEnvelope(
			"this program has nothing wired to answer a show, so it cannot open a result or a record for the screen; read the task with the tasks command instead"))
	}
	if !attached.takeAShowSlot() {
		return attached.write(errorEnvelope(fmt.Sprintf(
			"this screen already has %d shows waiting to be answered, which is as many as the socket holds, so wait for one to come back before asking again",
			MaxShowsInFlight)))
	}
	go socket.answerShow(attached, envelope.Fields)
	return nil
}

// answerShow runs the hook under the show deadline and writes what it answered,
// or the error it gave, or the word that it took too long, to the screen that
// asked. The request's fields ride on an error too, so a screen with several
// shows out knows which one failed, and an answer with no type is sent as a
// shown, which is the one type a show is answered with. The hook's context ends
// when the screen hangs up, so nothing runs on for a screen that is gone.
func (socket *Socket) answerShow(attached *client, fields map[string]string) {
	defer attached.freeAShowSlot()
	deadline := showDeadlineFor(socket.options.AnswerDeadline)
	ctx, giveUp := context.WithTimeout(attached.ctx, deadline)
	defer giveUp()

	answer, err := socket.callShow(ctx, fields, deadline)
	switch {
	case err != nil:
		answer = errorEnvelope(err.Error())
		answer.Fields = fields
	case answer.Type == "":
		answer.Type = contract.SocketShown
	}
	if attached.ctx.Err() != nil {
		return
	}
	if err := attached.write(answer); err != nil {
		attached.close()
	}
}

// callShow runs the hook and comes back with its answer, or, when the deadline
// passes first, with an error saying how long the socket waited. The hook goes
// on to its own end with a cancelled context, and its late answer is dropped.
func (socket *Socket) callShow(ctx context.Context, fields map[string]string, deadline time.Duration) (contract.SocketEnvelope, error) {
	type answered struct {
		envelope contract.SocketEnvelope
		err      error
	}
	done := make(chan answered, 1)
	go func() {
		envelope, err := socket.options.Show(ctx, fields)
		done <- answered{envelope: envelope, err: err}
	}()

	select {
	case came := <-done:
		return came.envelope, came.err
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return contract.SocketEnvelope{}, fmt.Errorf(
				"the show was not answered within %s, so try it again once the agent is quieter", deadline)
		}
		return contract.SocketEnvelope{}, errors.New("the screen hung up before the show was answered, so there is nobody to show it to")
	}
}

// showDeadlineFor is how long a show may take: the socket's own answer deadline
// when the user set one, and DefaultShowDeadline when the caps left it at none.
func showDeadlineFor(answerDeadline time.Duration) time.Duration {
	if answerDeadline <= 0 {
		return DefaultShowDeadline
	}
	return answerDeadline
}

// takeAShowSlot counts one more show in flight for this screen, and says no
// when the screen is closed or already holds as many as the cap allows.
func (attached *client) takeAShowSlot() bool {
	attached.guard.Lock()
	defer attached.guard.Unlock()
	if attached.closed || attached.showsInFlight >= MaxShowsInFlight {
		return false
	}
	attached.showsInFlight++
	return true
}

// freeAShowSlot counts one show answered.
func (attached *client) freeAShowSlot() {
	attached.guard.Lock()
	defer attached.guard.Unlock()
	attached.showsInFlight--
}
