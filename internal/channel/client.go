package channel

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// client is one screen connected to the socket: a connection, the reader that
// takes what it sends, and, once it has attached, its own place in the event
// stream.
type client struct {
	socket     *Socket
	connection net.Conn
	ctx        context.Context
	stop       context.CancelFunc

	writing sync.Mutex

	guard        sync.Mutex
	subscription *Subscription
	closed       bool
}

// newClient wraps one accepted connection.
func newClient(socket *Socket, connection net.Conn) *client {
	ctx, stop := context.WithCancel(context.Background())
	return &client{socket: socket, connection: connection, ctx: ctx, stop: stop}
}

// read takes one line at a time from the screen until it hangs up, sends
// something the socket cannot read, or sends a line past the cap. A line that is
// only whitespace is passed over, because a blank line is not a message.
func (attached *client) read() {
	defer attached.close()

	lines := bufio.NewScanner(attached.connection)
	lines.Buffer(make([]byte, 0, firstLineBytes), attached.socket.options.MaxLineBytes)
	for lines.Scan() {
		if len(strings.TrimSpace(lines.Text())) == 0 {
			continue
		}
		envelope, err := decodeFromScreen(lines.Bytes())
		if err != nil {
			_ = attached.write(errorEnvelope(err.Error()))
			return
		}
		if err := attached.socket.handle(attached, envelope); err != nil {
			return
		}
	}
	if err := lines.Err(); err != nil {
		attached.reportReadTrouble(err)
	}
}

// reportReadTrouble tells the screen why the socket stopped reading it, which is
// worth saying only when the line was too long; anything else means the
// connection is already gone.
func (attached *client) reportReadTrouble(err error) {
	if !errors.Is(err, bufio.ErrTooLong) {
		return
	}
	_ = attached.write(errorEnvelope(fmt.Sprintf(
		"that line is longer than the %d bytes the socket reads, so send what you typed rather than a file",
		attached.socket.options.MaxLineBytes)))
}

// write sends one message to this screen, with every piece of its text put
// through the vault's redactor first, because no secret leaves the program.
func (attached *client) write(envelope contract.SocketEnvelope) error {
	redacted := attached.socket.redact(envelope)

	attached.writing.Lock()
	defer attached.writing.Unlock()
	if err := attached.connection.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
		return fmt.Errorf("cannot put a deadline on the write to a screen: %w", err)
	}
	return contract.EncodeSocketEnvelope(attached.connection, redacted)
}

// close hangs up on this screen and takes it off the stream and off the socket.
// Closing a client that is already closed does nothing.
func (attached *client) close() {
	attached.guard.Lock()
	if attached.closed {
		attached.guard.Unlock()
		return
	}
	attached.closed = true
	subscription := attached.subscription
	attached.subscription = nil
	attached.guard.Unlock()

	if subscription != nil {
		subscription.Close()
	}
	attached.stop()
	_ = attached.connection.Close()
	attached.socket.forget(attached)
}

// reading says whether this screen has attached and is being sent the event
// stream.
func (attached *client) reading() bool {
	attached.guard.Lock()
	defer attached.guard.Unlock()
	return attached.subscription != nil
}

// attach puts this screen on the event stream and starts sending it what the
// loop publishes. Attaching twice does nothing the second time.
func (attached *client) attach() error {
	attached.guard.Lock()
	if attached.closed || attached.subscription != nil {
		attached.guard.Unlock()
		return nil
	}
	subscription, err := attached.socket.options.Stream.Subscribe()
	if err != nil {
		attached.guard.Unlock()
		return attached.write(errorEnvelope(err.Error()))
	}
	attached.subscription = subscription
	attached.guard.Unlock()

	go attached.forward(subscription)
	return nil
}

// detach takes this screen off the event stream and leaves it connected.
func (attached *client) detach() {
	attached.guard.Lock()
	subscription := attached.subscription
	attached.subscription = nil
	attached.guard.Unlock()
	if subscription != nil {
		subscription.Close()
	}
}

// forward sends everything on this screen's subscription to it, and hangs up on
// a screen that fell so far behind that the stream dropped it.
func (attached *client) forward(subscription *Subscription) {
	for envelope := range subscription.Events() {
		if err := attached.write(envelope); err != nil {
			attached.close()
			return
		}
	}
	if subscription.Dropped() {
		_ = attached.write(errorEnvelope(
			"this screen fell too far behind the agent to keep up, so it was let go; attach again to carry on"))
		attached.close()
	}
}

// handle does what one message from a screen asks for. It returns an error only
// when this screen can no longer be written to, which is what ends its
// connection; everything else a screen gets wrong is answered and forgiven.
func (socket *Socket) handle(attached *client, envelope contract.SocketEnvelope) error {
	switch envelope.Type {
	case contract.SocketAttach:
		return attached.attach()
	case contract.SocketDetach:
		attached.detach()
		return nil
	case contract.SocketMessage, contract.SocketCommand:
		return socket.enqueue(attached, envelope)
	case contract.SocketApprove:
		return socket.answerPreview(attached, envelope, approvalIn(envelope))
	case contract.SocketDeny:
		return socket.answerPreview(attached, envelope, contract.AnswerReject)
	case contract.SocketSecret:
		return socket.answerPrompt(attached, envelope)
	default:
		// The line reader refuses every other type before it reaches here, so
		// this can only happen if the two ever drift apart, and then doing
		// nothing is the safe thing to do.
		return nil
	}
}

// enqueue writes one message or command from a screen into the queue, which is
// the one path to the loop and the one that survives a restart, and then hands a
// copy to whoever is watching what this channel receives.
func (socket *Socket) enqueue(attached *client, envelope contract.SocketEnvelope) error {
	text := strings.TrimSpace(envelope.Text)
	if envelope.Type == contract.SocketCommand && text != "" && !strings.HasPrefix(text, CommandPrefix) {
		text = CommandPrefix + text
	}
	message := contract.Inbound{
		ID:          envelope.ID,
		Sender:      TerminalChannelName,
		Text:        text,
		Attachments: envelope.Attachments,
		Received:    socket.options.Clock.Now(),
		Channel:     TerminalChannelName,
	}

	if _, err := socket.options.Queue.Add(attached.ctx, message); err != nil {
		return attached.write(errorEnvelope(err.Error()))
	}
	socket.deliver(message)
	return nil
}

// answerPreview hands a screen's approve or deny to whoever is waiting on that
// preview, and tells the screen when there is nothing waiting under that id.
func (socket *Socket) answerPreview(attached *client, envelope contract.SocketEnvelope, answer contract.PreviewAnswer) error {
	socket.guard.Lock()
	waiting, held := socket.previews[envelope.ID]
	socket.guard.Unlock()
	if !held {
		return attached.write(errorEnvelope(fmt.Sprintf(
			"there is nothing waiting to be approved or denied under the number %q, so check the number on the preview",
			shortenedText(envelope.ID))))
	}

	select {
	case waiting <- answer:
	default:
	}
	return nil
}

// answerPrompt hands a screen's secret to whoever is waiting on that masked
// prompt. The secret is never written anywhere: it goes straight to the caller
// that asked for it.
func (socket *Socket) answerPrompt(attached *client, envelope contract.SocketEnvelope) error {
	socket.guard.Lock()
	waiting, held := socket.prompts[envelope.ID]
	socket.guard.Unlock()
	if !held {
		return attached.write(errorEnvelope(fmt.Sprintf(
			"there is nothing waiting for a secret under the number %q, so check the number on the prompt",
			shortenedText(envelope.ID))))
	}

	select {
	case waiting <- envelope.Secret:
	default:
	}
	return nil
}

// approvalIn reads which of the two yeses a screen sent: this once, or always
// for the rest of the session, which the screen says by putting
// contract.ApproveAlwaysText in the approve's text.
func approvalIn(envelope contract.SocketEnvelope) contract.PreviewAnswer {
	if strings.EqualFold(strings.TrimSpace(envelope.Text), contract.ApproveAlwaysText) {
		return contract.AnswerAlways
	}
	return contract.AnswerOnce
}

// decodeFromScreen reads one line the way the socket reads every line: it has to
// be one JSON object, and its type has to be one a screen sends.
func decodeFromScreen(line []byte) (contract.SocketEnvelope, error) {
	envelope, err := contract.DecodeSocketEnvelope(line)
	if err != nil {
		return contract.SocketEnvelope{}, err
	}
	if !envelope.Type.FromScreen() {
		return contract.SocketEnvelope{}, fmt.Errorf(
			"the socket message type %q is one only the agent sends, so send a message, a command, an approve, a deny, a secret, an attach, or a detach",
			envelope.Type)
	}
	return envelope, nil
}
