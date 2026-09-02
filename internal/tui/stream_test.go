package tui

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

// send hands the screen one message as though it had come off the socket.
func send(screen *Screen, envelope contract.SocketEnvelope) {
	screen.Update(envelopeMessage{envelope: envelope})
}

// replyTexts is the text of every reply block in the transcript, in order.
func replyTexts(screen *Screen) []string {
	texts := []string{}
	for _, item := range screen.blocks {
		if item.kind == blockReply {
			texts = append(texts, item.text)
		}
	}
	return texts
}

func TestStreamedDeltasCoalesceIntoOneReplyBlock(t *testing.T) {
	screen, clock := newTestScreen(80, 24)
	pieces := []string{"Nine years ", "ago today ", "DigiByte was born."}
	for _, piece := range pieces {
		send(screen, contract.SocketEnvelope{Type: contract.SocketDelta, Text: piece})
	}

	if strings.Contains(screen.View(), "Nine years") {
		t.Error("a delta reached the frame before the thirty milliseconds were up, and deltas are coalesced so the terminal is not repainted per word")
	}

	advance(screen, clock, heartbeatInterval)
	texts := replyTexts(screen)
	if len(texts) != 1 {
		t.Fatalf("three deltas made %d reply blocks, and they are one reply", len(texts))
	}
	if wanted := strings.Join(pieces, ""); texts[0] != wanted {
		t.Errorf("the reply reads %q, and the deltas joined together read %q", texts[0], wanted)
	}
	if !strings.Contains(screen.View(), "DigiByte was born.") {
		t.Error("the reply is not on the frame after the deltas were flushed")
	}
}

func TestAFinishedReplyClosesTheBlockAndTheNextDeltaStartsANewOne(t *testing.T) {
	screen, clock := newTestScreen(80, 24)
	send(screen, contract.SocketEnvelope{Type: contract.SocketDelta, Text: "the first "})
	advance(screen, clock, heartbeatInterval)
	send(screen, contract.SocketEnvelope{Type: contract.SocketReply, Text: "the first reply"})
	advance(screen, clock, heartbeatInterval)
	send(screen, contract.SocketEnvelope{Type: contract.SocketDelta, Text: "the second reply"})
	advance(screen, clock, heartbeatInterval)

	texts := replyTexts(screen)
	if len(texts) != 2 {
		t.Fatalf("there are %d reply blocks, and a finished reply followed by a new stream makes two", len(texts))
	}
	if texts[0] != "the first reply" {
		t.Errorf("the finished reply reads %q, and the program said %q", texts[0], "the first reply")
	}
	if texts[1] != "the second reply" {
		t.Errorf("the reply after it reads %q, and the delta said %q", texts[1], "the second reply")
	}
}

func TestAReplyWithNoDeltasBeforeItStillMakesABlock(t *testing.T) {
	screen, clock := newTestScreen(80, 24)
	send(screen, contract.SocketEnvelope{Type: contract.SocketReply, Text: "a whole reply at once"})
	advance(screen, clock, heartbeatInterval)

	texts := replyTexts(screen)
	if len(texts) != 1 || texts[0] != "a whole reply at once" {
		t.Fatalf("the reply blocks are %q, and one reply makes one block holding it", texts)
	}
}

func TestMarkdownInAReplyIsRenderedLightly(t *testing.T) {
	screen, _ := newTestScreen(80, 24)
	screen.remember(block{kind: blockReply, text: "# A heading\nplain **bold** and `code`\n* an item"})

	frame := screen.View()
	for _, wanted := range []string{"A heading", "plain bold and code", "- an item"} {
		if !strings.Contains(frame, wanted) {
			t.Errorf("the frame does not hold %q, and markdown is rendered lightly rather than shown as written", wanted)
		}
	}
	for _, unwanted := range []string{"# A heading", "**bold**", "`code`"} {
		if strings.Contains(frame, unwanted) {
			t.Errorf("the frame still holds the markdown %q, and the marks are drawn as style rather than as text", unwanted)
		}
	}
}
