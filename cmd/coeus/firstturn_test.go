package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/channel"
	workingcontext "github.com/JaredTate/coeus/internal/context"
	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// aTurnWithOneScriptedReply builds the first-turn stand-in over a temporary home,
// a scripted model, a fake terminal channel, and a stream a test can listen to.
func aTurnWithOneScriptedReply(t *testing.T, script testkit.Script) (*firstTurn, *testkit.FakeChannel, *testkit.FakeStore, *channel.Stream) {
	t.Helper()
	home := testkit.NewTempHome(t)
	if err := os.MkdirAll(home.PersonaFolder(), contract.HomeFolderMode); err != nil {
		t.Fatalf("making the persona folder failed: %v", err)
	}
	for path, text := range map[string]string{
		home.SoulFile():       "I am Coeus, and I answer plainly.",
		home.UserFactsFile():  "The user is Jared.",
		home.WorldFactsFile(): "DigiByte launched in 2014.",
	} {
		if err := os.WriteFile(path, []byte(text), contract.DataFileMode); err != nil {
			t.Fatalf("writing %s failed: %v", path, err)
		}
	}

	where := testkit.NewFakeChannel(contract.TerminalChannelName)
	eventLog := testkit.NewFakeStore()
	stream := channel.NewStream(channel.StreamOptions{})
	t.Cleanup(stream.Close)

	settings := contract.DefaultConfig()
	builder, err := workingcontext.New(workingcontext.Options{
		Home:            home,
		MemoryCaps:      settings.MemoryCaps,
		MaxOutputTokens: settings.Caps.OutputTokensPerCall,
	})
	if err != nil {
		t.Fatalf("building the working context builder failed: %v", err)
	}

	turn := &firstTurn{
		settings: settings,
		builder:  builder,
		model:    testkit.NewFakeModel(script),
		stream:   stream,
		eventLog: eventLog,
		talk:     &conversation{},
		channels: func(name string) (contract.Channel, bool) {
			if name == where.Name() {
				return where, true
			}
			return nil, false
		},
	}
	return turn, where, eventLog, stream
}

// oneScriptedReply is a script that answers once with the text.
func oneScriptedReply(text string, expect ...string) testkit.Script {
	return testkit.Script{
		Name:          "local",
		ContextLength: 24000,
		Steps: []testkit.Step{{
			Expect: expect,
			Text:   text,
			Finish: contract.FinishEnd,
			Usage:  contract.Usage{InputTokens: 200, OutputTokens: 7},
		}},
	}
}

func TestTheRequestIsTheOneInternalContextBuilds(t *testing.T) {
	turn, _, _, _ := aTurnWithOneScriptedReply(t, oneScriptedReply("done"))
	turn.talk.add(contract.Message{Role: contract.RoleUser, Text: "when did DigiByte launch?"})

	request, err := turn.request(context.Background())
	if err != nil {
		t.Fatalf("building the request failed: %v", err)
	}

	if len(request.SystemBlocks) < 2 {
		t.Fatalf("the request carries %d system blocks, want at least the instructions and the persona", len(request.SystemBlocks))
	}
	if request.SystemBlocks[0].Name != workingcontext.BlockInstructions {
		t.Errorf("the first block is %q, want %q", request.SystemBlocks[0].Name, workingcontext.BlockInstructions)
	}
	if request.SystemBlocks[0].Text != workingcontext.InstructionText {
		t.Error("the first block is not the instruction text internal/context holds, so the person is not reading the real prompt")
	}
	if request.SystemBlocks[1].Name != workingcontext.BlockPersona {
		t.Errorf("the second block is %q, want %q", request.SystemBlocks[1].Name, workingcontext.BlockPersona)
	}
	if !strings.Contains(request.SystemBlocks[1].Text, "I am Coeus, and I answer plainly.") {
		t.Errorf("the persona block does not hold the persona files:\n%s", request.SystemBlocks[1].Text)
	}
	if !request.ToolsOff || len(request.Tools) != 0 {
		t.Error("the request offers tools, and nothing runs a tool until the turn loop lands")
	}
	if len(request.Messages) == 0 {
		t.Error("the request carries no messages, so the model would be asked nothing")
	}
}

func TestTheConversationKeepsTheNewestMessagesAndStaysUnderItsCap(t *testing.T) {
	talk := &conversation{}

	for at := 0; at < maxConversationMessages+5; at++ {
		talk.add(contract.Message{Role: contract.RoleUser, Text: "message " + string(rune('a'+at%26))})
	}
	talk.add(contract.Message{Role: contract.RoleAssistant, Text: "the newest one"})

	kept := talk.soFar()
	if len(kept) > maxConversationMessages {
		t.Errorf("the conversation holds %d messages, and the cap is %d", len(kept), maxConversationMessages)
	}
	if kept[len(kept)-1].Text != "the newest one" {
		t.Errorf("the newest message was dropped; the last one is %q", kept[len(kept)-1].Text)
	}
}

func TestTheFirstTurnSendsTheReplyOnceAndLogsBothSides(t *testing.T) {
	turn, where, eventLog, stream := aTurnWithOneScriptedReply(t,
		oneScriptedReply("DigiByte launched in 2014.", "when did DigiByte launch"))
	listening, err := stream.Subscribe()
	if err != nil {
		t.Fatalf("subscribing to the stream failed: %v", err)
	}
	defer listening.Close()

	ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	err = turn.StartTask(ctx, contract.Inbound{
		ID:       "1",
		Sender:   contract.TerminalChannelName,
		Channel:  contract.TerminalChannelName,
		Text:     "when did DigiByte launch?",
		Received: time.Now(),
	})
	if err != nil {
		t.Fatalf("the first turn failed: %v", err)
	}

	sent := where.Sent()
	if len(sent) != 1 || sent[0] != "DigiByte launched in 2014." {
		t.Errorf("the channel carried %v, want the one reply the script wrote", sent)
	}
	// The answer must reach a screen once and once only. A delta carrying the
	// same words as the reply arrives on the event stream, which is not ordered
	// against the channel the reply goes out on, and the terminal draws the
	// answer a second time when it lands after the reply.
	if deltas := textOfDeltas(listening); deltas != "" {
		t.Errorf("deltas carrying %q went out beside the reply, so the screen would draw the answer twice", deltas)
	}
	kinds := loggedKinds(t, eventLog)
	if !strings.Contains(kinds, string(contract.EventMessage)) || !strings.Contains(kinds, string(contract.EventReply)) {
		t.Errorf("the event log holds %s, want both the message and the reply", kinds)
	}
}

func TestTheFirstTurnTellsTheUserWhenTheModelCannotBeReached(t *testing.T) {
	turn, where, _, _ := aTurnWithOneScriptedReply(t, testkit.Script{Name: "local", ContextLength: 24000})

	ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	err := turn.StartTask(ctx, contract.Inbound{
		Channel: contract.TerminalChannelName,
		Text:    "anything at all",
	})

	if err == nil {
		t.Fatal("a model that ran out of script answered anyway, want an error saying what went wrong")
	}
	if len(where.Sent()) != 1 || !strings.Contains(where.Sent()[0], "could not") {
		t.Errorf("the user was told %v, want one line saying the model could not be reached", where.Sent())
	}
}

// textOfDeltas drains what is waiting on a subscription and joins the text of
// every delta in it.
func textOfDeltas(listening *channel.Subscription) string {
	written := strings.Builder{}
	for {
		select {
		case envelope, open := <-listening.Events():
			if !open {
				return written.String()
			}
			if envelope.Type == contract.SocketDelta {
				written.WriteString(envelope.Text)
			}
		default:
			return written.String()
		}
	}
}

// loggedKinds is the kinds of every event written to the log, joined for a
// failure message.
func loggedKinds(t *testing.T, eventLog *testkit.FakeStore) string {
	t.Helper()
	kinds := []string{}
	err := eventLog.Replay(context.Background(), func(event contract.Event) error {
		kinds = append(kinds, string(event.Kind))
		return nil
	})
	if err != nil {
		t.Fatalf("reading the event log back failed: %v", err)
	}
	return strings.Join(kinds, ", ")
}
