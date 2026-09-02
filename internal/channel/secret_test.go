package channel

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// theSecret is what the user types at the masked prompt in these tests. It is
// nowhere else in the package, so a test can search anything the socket wrote
// for it and know that finding it means the socket leaked it.
const theSecret = "correct-horse-battery-staple-9182"

// aQuietWait is how long a test watches a screen that should be sent nothing at
// all before it is satisfied that nothing came.
const aQuietWait = 250 * time.Millisecond

// secretResult is what one call to AskSecret came back with.
type secretResult struct {
	secret string
	err    error
}

// askSecret starts one masked prompt in the background and hands back where its
// answer will arrive.
func (harness *socketHarness) askSecret(ctx context.Context) chan secretResult {
	answers := make(chan secretResult, 1)
	go func() {
		secret, err := harness.socket.AskSecret(ctx, "the password for x-account")
		answers <- secretResult{secret: secret, err: err}
	}()
	return answers
}

// nothingArrives says whether the screen was sent nothing at all for a while.
func (client *screen) nothingArrives() bool {
	client.t.Helper()
	if err := client.socket.SetReadDeadline(time.Now().Add(aQuietWait)); err != nil {
		client.t.Fatalf("cannot put a deadline on the socket: %v", err)
	}
	line, err := client.lines.ReadBytes('\n')
	if err == nil {
		client.t.Logf("the screen was sent %s", line)
	}
	return err != nil
}

func TestAMaskedPromptComesBackWithTheSecretAndLeavesItNowhereElse(t *testing.T) {
	harness := newSocketHarness(t)
	client := harness.attach(t)
	watching, err := harness.socket.Receive(context.Background())
	if err != nil {
		t.Fatalf("watching what the socket receives failed: %v", err)
	}
	answers := harness.askSecret(context.Background())

	asked := client.next()
	if asked.Type != contract.SocketAsk {
		t.Fatalf("the screen saw a %s, want a question", asked.Type)
	}
	if asked.Fields[SecretPromptField] != SecretPromptValue {
		t.Errorf("the question arrived with the fields %v, and it has to be marked a masked prompt so the screen hides what is typed", asked.Fields)
	}
	if asked.ID == "" {
		t.Fatal("the question arrived without a number, and there is no way to answer it")
	}
	if asked.Text != "the password for x-account" {
		t.Errorf("the question reads %q, want the prompt that was asked", asked.Text)
	}

	client.send(contract.SocketEnvelope{Type: contract.SocketSecret, ID: asked.ID, Secret: theSecret})
	got := <-answers
	if got.err != nil {
		t.Fatalf("asking for the secret failed: %v", got.err)
	}
	if got.secret != theSecret {
		t.Errorf("the caller was given %q, want what the user typed", got.secret)
	}

	if !client.nothingArrives() {
		t.Error("the screen was sent something after it answered the masked prompt")
	}
	held, err := harness.queue.Held(context.Background())
	if err != nil {
		t.Fatalf("counting the queue failed: %v", err)
	}
	if held != 0 {
		t.Errorf("the queue holds %d messages, and a secret must never be written down", held)
	}
	select {
	case message := <-watching:
		t.Errorf("the secret came through as a message: %+v", message)
	default:
	}
}

func TestASecondScreenNeverSeesWhatWasTypedAtTheMaskedPrompt(t *testing.T) {
	harness := newSocketHarness(t)
	typing := harness.attach(t)
	watching := harness.dial(t)
	watching.send(contract.SocketEnvelope{Type: contract.SocketAttach})
	harness.waitForAttached(t, 2)
	answers := harness.askSecret(context.Background())

	asked := typing.next()
	if question := watching.next(); question.Type != contract.SocketAsk {
		t.Fatalf("the second screen saw a %s, want the same question", question.Type)
	}
	typing.send(contract.SocketEnvelope{Type: contract.SocketSecret, ID: asked.ID, Secret: theSecret})

	if got := <-answers; got.secret != theSecret {
		t.Fatalf("the caller was given %q, want what the user typed", got.secret)
	}
	if !watching.nothingArrives() {
		t.Error("the second screen was sent something after the secret was typed on the first")
	}
}

func TestNothingTheSocketSendsOutEverCarriesASecret(t *testing.T) {
	harness := newSocketHarness(t)
	client := harness.attach(t)

	if err := harness.socket.Send(context.Background(), "here is the reply"); err != nil {
		t.Fatalf("sending failed: %v", err)
	}
	// The reply is written straight to the screen, so the only way a secret
	// could ride out is in the field made for it, which is always emptied.
	sent := harness.socket.redact(contract.SocketEnvelope{
		Type:   contract.SocketReply,
		Text:   "here is the reply",
		Secret: theSecret,
	})
	if sent.Secret != "" {
		t.Errorf("a message on its way to a screen still carried the secret %q", sent.Secret)
	}
	if got := client.next(); strings.Contains(got.Text, theSecret) {
		t.Errorf("the screen was sent %q, which holds the secret", got.Text)
	}
}

func TestAMaskedPromptWithNoScreenAttachedSaysToUseTheTerminal(t *testing.T) {
	harness := newSocketHarness(t)
	harness.dial(t)

	secret, err := harness.socket.AskSecret(context.Background(), "the password for x-account")
	if !errors.Is(err, contract.ErrNoMaskedPrompt) {
		t.Fatalf("asking with nobody attached failed with %v, want the no-masked-prompt error", err)
	}
	if secret != "" {
		t.Errorf("a secret came back from nobody: %q", secret)
	}
}

func TestAMaskedPromptNobodyAnswersInTimeGivesUp(t *testing.T) {
	harness := newSocketHarness(t)
	client := harness.attach(t)
	answers := harness.askSecret(context.Background())

	if asked := client.next(); asked.Type != contract.SocketAsk {
		t.Fatalf("the screen saw a %s, want a question", asked.Type)
	}
	harness.waitForSleepers(t, 1)
	harness.clock.Advance(DefaultAnswerDeadline + time.Second)

	got := <-answers
	if got.err == nil {
		t.Fatal("a masked prompt nobody answered came back with no error")
	}
	if got.secret != "" {
		t.Errorf("a secret came back from a prompt nobody answered: %q", got.secret)
	}
}

func TestTwoMaskedPromptsGetTheirOwnNumbers(t *testing.T) {
	harness := newSocketHarness(t)
	client := harness.attach(t)

	first := harness.askSecret(context.Background())
	firstAsked := client.next()
	second := harness.askSecret(context.Background())
	secondAsked := client.next()
	if firstAsked.ID == secondAsked.ID {
		t.Fatalf("both prompts arrived under the number %q, and neither could be answered on its own", firstAsked.ID)
	}

	client.send(contract.SocketEnvelope{Type: contract.SocketSecret, ID: secondAsked.ID, Secret: theSecret})
	if got := <-second; got.secret != theSecret {
		t.Errorf("the second prompt was given %q, want what the user typed", got.secret)
	}
	client.send(contract.SocketEnvelope{Type: contract.SocketSecret, ID: firstAsked.ID, Secret: "the other one"})
	if got := <-first; got.secret != "the other one" {
		t.Errorf("the first prompt was given %q, want what the user typed", got.secret)
	}
}

func TestTheSocketPassesTheChannelContractCheck(t *testing.T) {
	harness := newSocketHarness(t)
	client := harness.attach(t)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	// One screen stands in for the terminal and answers whatever the check asks.
	answering := make(chan error, 1)
	go func() { answering <- answerLikeATerminal(client) }()

	if err := testkit.CheckChannel(ctx, harness.socket); err != nil {
		t.Fatalf("the socket does not keep the channel contract: %v", err)
	}
	if err := <-answering; err != nil {
		t.Fatalf("the screen standing in for the terminal gave up: %v", err)
	}
}

// answerLikeATerminal answers the one preview and the one masked prompt the
// channel contract check asks, the way a person at the terminal would. It reads
// past the reply and the file the check sends first, and it never fails the test
// itself, because it runs on a goroutine of its own.
func answerLikeATerminal(client *screen) error {
	answered := 0
	for range maxMessagesInTheContractCheck {
		asked, err := client.tryNext()
		if err != nil {
			return err
		}
		switch asked.Type {
		case contract.SocketPreview:
			client.send(contract.SocketEnvelope{Type: contract.SocketApprove, ID: asked.ID})
			answered++
		case contract.SocketAsk:
			client.send(contract.SocketEnvelope{Type: contract.SocketSecret, ID: asked.ID, Secret: theSecret})
			answered++
		}
		if answered == 2 {
			return nil
		}
	}
	return errors.New("the contract check sent more messages than the terminal was ready to answer, so look at what it now sends")
}

// maxMessagesInTheContractCheck is how many messages the check may send before
// the screen standing in for the terminal gives up on it.
const maxMessagesInTheContractCheck = 8
