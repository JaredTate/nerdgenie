package channel

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// TestTheFirstPieceOfAReplyReachesEveryScreenAtOnce holds the design's promise
// that streaming looks instant: the first piece goes out the moment it arrives,
// and later pieces follow once the gathering window has passed.
func TestTheFirstPieceOfAReplyReachesEveryScreenAtOnce(t *testing.T) {
	harness := newSocketHarness(t)
	first := harness.attach(t)
	second := harness.attach(t)

	if err := harness.socket.SendDelta(context.Background(), "Hello "); err != nil {
		t.Fatalf("sending the first piece failed: %v", err)
	}
	for _, client := range []*screen{first, second} {
		got := client.next()
		if got.Type != contract.SocketDelta || got.Text != "Hello " || got.Reset {
			t.Errorf("a screen was sent %+v, want the first piece as a delta", got)
		}
	}
	harness.clock.Advance(deltaCoalesce)
	if err := harness.socket.SendDelta(context.Background(), "world"); err != nil {
		t.Fatalf("sending the second piece failed: %v", err)
	}
	if got := first.next(); got.Text != "world" {
		t.Errorf("the screen was sent %q after the window passed, want the second piece", got.Text)
	}
}

// TestPiecesAreGatheredForThirtyMillisecondsBeforeTheyGoOut pins the window
// the design names and proves pieces inside it are joined into one message.
func TestPiecesAreGatheredForThirtyMillisecondsBeforeTheyGoOut(t *testing.T) {
	if deltaCoalesce != 30*time.Millisecond {
		t.Fatalf("the gathering window is %s, and the design says thirty milliseconds", deltaCoalesce)
	}
	harness := newSocketHarness(t)
	client := harness.attach(t)
	ctx := context.Background()

	_ = harness.socket.SendDelta(ctx, "a")
	client.next()
	for _, piece := range []string{"b", "c"} {
		if err := harness.socket.SendDelta(ctx, piece); err != nil {
			t.Fatalf("sending %q failed: %v", piece, err)
		}
	}
	harness.clock.Advance(deltaCoalesce)
	if err := harness.socket.SendDelta(ctx, "d"); err != nil {
		t.Fatalf("sending the last piece failed: %v", err)
	}
	if got := client.next(); got.Text != "bcd" {
		t.Errorf("the screen was sent %q, want the three pieces inside the window joined as one", got.Text)
	}
}

// TestASecretSplitAcrossTwoPiecesNeverReachesAScreen is the hole the Signal
// channel closed for whole messages, closed here for the pieces of a reply: the
// end of the text waits until enough has arrived to know it is not a secret.
func TestASecretSplitAcrossTwoPiecesNeverReachesAScreen(t *testing.T) {
	harness := newSocketHarness(t)
	harness.secrets.Add("x-account", contract.Credential{
		Site: "x-account", Username: "jared", Password: "hunter2the-real-one",
	})
	client := harness.attach(t)
	ctx := context.Background()

	for _, piece := range []string{"the password is hunter2the-", "real-one and that is all there is to say about it, so we go on"} {
		if err := harness.socket.SendDelta(ctx, piece); err != nil {
			t.Fatalf("sending %q failed: %v", piece, err)
		}
		harness.clock.Advance(deltaCoalesce)
	}
	if err := harness.socket.Send(ctx, "the password is hunter2the-real-one and that is all there is to say about it, so we go on"); err != nil {
		t.Fatalf("sending the reply failed: %v", err)
	}
	seen := ""
	for {
		got := client.next()
		if strings.Contains(got.Text, "hunter2the-real-one") {
			t.Fatalf("a screen was sent %q, and no secret may leave the program in pieces either", got.Text)
		}
		if got.Type == contract.SocketReply {
			break
		}
		seen += got.Text
	}
	if !strings.Contains(seen, contract.RedactedMarker) {
		t.Errorf("the pieces read %q, want the secret replaced by %q before the reply came", seen, contract.RedactedMarker)
	}
}

// TestAResetWithdrawsThePartialReply covers the retry: the screens are told to
// take the partial reply down, and the pieces after it start the reply over.
func TestAResetWithdrawsThePartialReply(t *testing.T) {
	harness := newSocketHarness(t)
	client := harness.attach(t)
	ctx := context.Background()

	_ = harness.socket.SendDelta(ctx, "Hello ")
	client.next()
	if err := harness.socket.ResetDelta(ctx); err != nil {
		t.Fatalf("withdrawing the partial reply failed: %v", err)
	}
	if got := client.next(); got.Type != contract.SocketDelta || !got.Reset {
		t.Errorf("the screen was sent %+v, want a delta that withdraws the reply", got)
	}
	if err := harness.socket.SendDelta(ctx, "Hello world"); err != nil {
		t.Fatalf("sending the reply over failed: %v", err)
	}
	if got := client.next(); got.Text != "Hello world" || got.Reset {
		t.Errorf("the screen was sent %+v, want the reply started over", got)
	}
}
