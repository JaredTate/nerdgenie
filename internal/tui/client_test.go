package tui

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// waitingLimit is how long a test waits for a goroutine to get somewhere before
// it gives up and fails, so that a broken client never hangs the suite.
const waitingLimit = 2 * time.Second

// fakeSocket is one link a test drives: it hands over the envelopes the test
// pushes into it, records everything the screen sends, and breaks when the test
// says so.
type fakeSocket struct {
	guard    sync.Mutex
	sent     []contract.SocketEnvelope
	failure  error
	arriving chan contract.SocketEnvelope
	broken   chan struct{}
	closed   bool
}

// newFakeSocket makes one open link.
func newFakeSocket() *fakeSocket {
	return &fakeSocket{
		arriving: make(chan contract.SocketEnvelope, 8),
		broken:   make(chan struct{}),
	}
}

// Receive hands over the next envelope, or the failure the test asked for.
func (socket *fakeSocket) Receive() (contract.SocketEnvelope, error) {
	select {
	case envelope := <-socket.arriving:
		return envelope, nil
	case <-socket.broken:
		socket.guard.Lock()
		defer socket.guard.Unlock()
		return contract.SocketEnvelope{}, socket.failure
	}
}

// Send records what the screen said.
func (socket *fakeSocket) Send(envelope contract.SocketEnvelope) error {
	socket.guard.Lock()
	defer socket.guard.Unlock()
	if socket.closed {
		return errors.New("this fake socket is closed, so nothing more can be sent on it")
	}
	socket.sent = append(socket.sent, envelope)
	return nil
}

// Close ends the link.
func (socket *fakeSocket) Close() error {
	socket.guard.Lock()
	defer socket.guard.Unlock()
	socket.closed = true
	return nil
}

// push makes an envelope arrive from the program.
func (socket *fakeSocket) push(envelope contract.SocketEnvelope) {
	socket.arriving <- envelope
}

// breakLink makes the link fail the way a program that has stopped would.
func (socket *fakeSocket) breakLink(why error) {
	socket.guard.Lock()
	socket.failure = why
	socket.guard.Unlock()
	close(socket.broken)
}

// everySent is a copy of what the screen has said on this link.
func (socket *fakeSocket) everySent() []contract.SocketEnvelope {
	socket.guard.Lock()
	defer socket.guard.Unlock()
	return append([]contract.SocketEnvelope{}, socket.sent...)
}

// fakeDialer opens fake sockets, or refuses, the way nothing listening on the
// real socket would.
type fakeDialer struct {
	guard  sync.Mutex
	tries  int
	refuse error
	opened chan *fakeSocket
}

// newFakeDialer makes a dialer that opens links.
func newFakeDialer() *fakeDialer {
	return &fakeDialer{opened: make(chan *fakeSocket, 8)}
}

// Dial opens one link, or refuses when the test asked it to.
func (dialer *fakeDialer) Dial(_ context.Context) (Connection, error) {
	dialer.guard.Lock()
	dialer.tries++
	refusing := dialer.refuse
	dialer.guard.Unlock()
	if refusing != nil {
		return nil, refusing
	}
	socket := newFakeSocket()
	dialer.opened <- socket
	return socket, nil
}

// timesTried is how many times the client has dialled.
func (dialer *fakeDialer) timesTried() int {
	dialer.guard.Lock()
	defer dialer.guard.Unlock()
	return dialer.tries
}

// refuseWith makes every dial from now on fail.
func (dialer *fakeDialer) refuseWith(why error) {
	dialer.guard.Lock()
	defer dialer.guard.Unlock()
	dialer.refuse = why
}

// nextLink waits for the client's next dial to open a link.
func (dialer *fakeDialer) nextLink(t *testing.T) *fakeSocket {
	t.Helper()
	select {
	case socket := <-dialer.opened:
		return socket
	case <-time.After(waitingLimit):
		t.Fatal("the client did not open a link, and it should have dialled by now")
		return nil
	}
}

// nextEvent waits for the next thing the client has to say.
func nextEvent(t *testing.T, events <-chan tea.Msg) tea.Msg {
	t.Helper()
	select {
	case message := <-events:
		return message
	case <-time.After(waitingLimit):
		t.Fatal("the client said nothing, and it should have reported by now")
		return nil
	}
}

// waitForSleeper waits until the client is asleep on the fake clock, which is
// how a test knows it is safe to move time on.
func waitForSleeper(t *testing.T, clock *testkit.FakeClock) {
	t.Helper()
	giveUpAt := time.Now().Add(waitingLimit)
	for time.Now().Before(giveUpAt) {
		if clock.Sleepers() > 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("the client never waited on the clock, and it must back off before dialling again")
}

func TestTheClientAttachesAndPassesOnWhatTheProgramSays(t *testing.T) {
	clock := testkitClock()
	dialer := newFakeDialer()
	client := NewClient(dialer, clock)
	events := client.Start()
	defer client.Close()

	socket := dialer.nextLink(t)
	if up, isLink := nextEvent(t, events).(linkMessage); !isLink || !up.up {
		t.Fatal("the client did not say the link was up once it had dialled")
	}
	sent := socket.everySent()
	if len(sent) != 1 || sent[0].Type != contract.SocketAttach {
		t.Fatalf("the client sent %+v on a new link, and the first thing it sends is an attach", sent)
	}

	socket.push(contract.SocketEnvelope{Type: contract.SocketReply, Text: "hello from the program"})
	carried, isEnvelope := nextEvent(t, events).(envelopeMessage)
	if !isEnvelope || carried.envelope.Text != "hello from the program" {
		t.Fatalf("the client passed on %+v, and the program said %q", carried, "hello from the program")
	}
}

func TestTheClientDialsAgainAfterTheLinkDrops(t *testing.T) {
	clock := testkitClock()
	dialer := newFakeDialer()
	client := NewClient(dialer, clock)
	events := client.Start()
	defer client.Close()

	socket := dialer.nextLink(t)
	nextEvent(t, events)
	socket.breakLink(errors.New("the program went away"))

	down, isLink := nextEvent(t, events).(linkMessage)
	if !isLink || down.up {
		t.Fatalf("the client said %+v when the link dropped, and it must say the link is down", down)
	}

	waitForSleeper(t, clock)
	if tried := dialer.timesTried(); tried != 1 {
		t.Fatalf("the client dialled %d times before waiting, and it must back off first", tried)
	}
	clock.Advance(firstReconnectWait)

	dialer.nextLink(t)
	if up, isLink := nextEvent(t, events).(linkMessage); !isLink || !up.up {
		t.Fatal("the client did not say the link was up again after it dialled a second time")
	}
}

func TestTheClientKeepsTryingWhenNothingIsListening(t *testing.T) {
	clock := testkitClock()
	dialer := newFakeDialer()
	dialer.refuseWith(errors.New("no program is listening on that socket"))
	client := NewClient(dialer, clock)
	events := client.Start()
	defer client.Close()

	if down, isLink := nextEvent(t, events).(linkMessage); !isLink || down.up {
		t.Fatal("the client did not report that it could not reach the program")
	}
	waitForSleeper(t, clock)
	clock.Advance(firstReconnectWait)
	waitForSleeper(t, clock)
	if tried := dialer.timesTried(); tried < 2 {
		t.Fatalf("the client dialled %d times, and it keeps trying so that the person never sees a crash", tried)
	}
}

func TestTheWaitBetweenTriesGrowsAndIsCapped(t *testing.T) {
	wait := firstReconnectWait
	for range 20 {
		wait = longerWait(wait)
	}
	if wait != maxReconnectWait {
		t.Errorf("the wait between tries grew to %s, and it is capped at %s", wait, maxReconnectWait)
	}
	if longerWait(firstReconnectWait) <= firstReconnectWait {
		t.Error("the wait between tries does not grow, and a program that is down would be dialled without pause")
	}
}

func TestSendingWithNoLinkSaysSoRatherThanLosingTheMessage(t *testing.T) {
	clock := testkitClock()
	dialer := newFakeDialer()
	dialer.refuseWith(errors.New("no program is listening on that socket"))
	client := NewClient(dialer, clock)
	client.Start()
	defer client.Close()

	err := client.Send(contract.SocketEnvelope{Type: contract.SocketMessage, Text: "hello"})
	if err == nil {
		t.Fatal("sending with no link said nothing went wrong, and the message went nowhere")
	}
	if !strings.Contains(err.Error(), "link") {
		t.Errorf("the error is %q, and it should say that there is no link yet", err)
	}
}

func TestTheStatusStripTellsTheTruthAboutTheLink(t *testing.T) {
	screen, _ := newTestScreen(80, 24)
	if !strings.Contains(screen.frame(), "connecting") {
		t.Error("the first frame does not say the screen is connecting")
	}

	screen.Update(linkMessage{up: true})
	if strings.Contains(screen.frame(), "connecting") {
		t.Error("the frame still says connecting after the link came up")
	}
	if !strings.Contains(screen.frame(), "healthy") {
		t.Error("the header does not show a healthy link once the screen has attached")
	}

	screen.Update(linkMessage{up: false, detail: "the program went away"})
	frame := screen.frame()
	if !strings.Contains(frame, "disconnected, reconnecting") {
		t.Error("the status strip does not say the link dropped, and it must say so at once")
	}
	if !strings.Contains(frame, "offline") {
		t.Error("the health dot does not show that there is no link")
	}
	typeWord(screen, "still typing")
	if screen.input.text() != "still typing" {
		t.Error("the input box stopped working when the link dropped, and it stays usable so the person can read back")
	}
}
