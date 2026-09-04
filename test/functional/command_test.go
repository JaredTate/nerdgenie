package functional

// This file is the functional test for the slash commands: a person types one
// into the terminal, it travels the same way a real one does, and the answer
// comes back on the same socket. The screen here is a bare Unix socket
// connection speaking the envelope format, because that is exactly what the
// terminal is; the loop between the queue and the registry is the smallest
// possible stand-in, and wave 3's loop replaces it without moving an assertion.

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/channel"
	"github.com/JaredTate/nerdgenie/internal/command"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// aCommandWait is how long anything in this file waits for the socket, the
// queue, or a reply before it says the agent is stuck.
const aCommandWait = 5 * time.Second

// messagesBeforeAReply is how many messages the screen reads while looking for
// the answer to a line it typed. The stream sends a status of its own now and
// then, and those are stepped over; anything more than a handful of them means
// the answer is not coming.
const messagesBeforeAReply = 8

// theStartOfTime is the moment the fake clock in this file starts at, so that
// nothing here reads the machine's own clock.
var theStartOfTime = time.Date(2026, time.September, 2, 14, 3, 0, 0, time.UTC)

// aTerminal is a running agent as far as a slash command can tell: the local
// socket, the queue behind it, the command registry, and one screen attached to
// it the way the terminal attaches.
type aTerminal struct {
	t        *testing.T
	socket   *channel.Socket
	queue    *channel.Queue
	registry *command.Registry
	screen   net.Conn
	lines    *bufio.Reader
	store    *testkit.FakeStore
}

func TestHelpStatusAndUndoAnswerThroughTheSocketTheTerminalUses(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	terminal := startATerminal(t, ctx)

	helped := terminal.typeLine(ctx, "/help")
	for _, wanted := range []string{"/help", "/status", "/undo"} {
		if !strings.Contains(helped, wanted) {
			t.Errorf("the help listing leaves out %s:\n%s", wanted, helped)
		}
	}

	reported := terminal.typeLine(ctx, "/status")
	for _, wanted := range []string{"model: local", "1200 tokens in", "jobs: 1 running", "channels:", contract.TerminalChannelName} {
		if !strings.Contains(reported, wanted) {
			t.Errorf("the status report leaves out %q:\n%s", wanted, reported)
		}
	}

	notes := terminal.aTurnThatRewroteAFile()
	undone := terminal.typeLine(ctx, "/undo")
	if !strings.Contains(undone, notes) {
		t.Errorf("the undo command does not say which file it put back:\n%s", undone)
	}
	back, err := os.ReadFile(notes)
	if err != nil {
		t.Fatalf("reading the file that was put back failed: %v", err)
	}
	if string(back) != "the words the user wrote" {
		t.Errorf("the file came back as %q rather than what it held before the turn", back)
	}
}

func TestALineNamingNoCommandComesBackAsOnePlainSentence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	terminal := startATerminal(t, ctx)

	taken := terminal.send(ctx, "/nothing-like-this")
	_, err := terminal.registry.Run(ctx, taken.Message.Text, contract.CommandContext{Channel: terminal.socket})
	if err == nil {
		t.Fatalf("the registry answered a command it does not hold")
	}
	if !strings.Contains(err.Error(), "/help") {
		t.Errorf("the refusal does not point at the help listing: %v", err)
	}
	terminal.finish(ctx, taken)
}

// typeLine sends one typed line the way the terminal sends it, runs it the way the
// loop will, and gives back the answer that reached the screen.
func (terminal *aTerminal) typeLine(ctx context.Context, line string) string {
	terminal.t.Helper()
	taken := terminal.send(ctx, line)

	answer, err := terminal.registry.Run(ctx, taken.Message.Text, contract.CommandContext{Channel: terminal.socket})
	if err != nil {
		terminal.t.Fatalf("running %q failed: %v", line, err)
	}
	if err := terminal.socket.Send(ctx, answer); err != nil {
		terminal.t.Fatalf("sending the answer to %q back failed: %v", line, err)
	}
	terminal.finish(ctx, taken)

	return terminal.replyText(line)
}

// replyText reads until the answer arrives, stepping over the status messages
// the stream sends a screen of its own accord, and gives back what it said.
func (terminal *aTerminal) replyText(line string) string {
	terminal.t.Helper()
	for read := 0; read < messagesBeforeAReply; read++ {
		envelope := terminal.next()
		if envelope.Type == contract.SocketStatus {
			continue
		}
		if envelope.Type != contract.SocketReply {
			terminal.t.Fatalf("the screen was sent a %s rather than a reply to %q", envelope.Type, line)
		}
		return envelope.Text
	}
	terminal.t.Fatalf("no reply to %q reached the screen in %d messages", line, messagesBeforeAReply)
	return ""
}

// send writes one command envelope and waits for it to reach the queue, which is
// the one path from a screen to the loop.
func (terminal *aTerminal) send(ctx context.Context, line string) channel.Queued {
	terminal.t.Helper()
	if err := contract.EncodeSocketEnvelope(terminal.screen, contract.SocketEnvelope{
		Type: contract.SocketCommand,
		Text: line,
	}); err != nil {
		terminal.t.Fatalf("sending %q failed: %v", line, err)
	}

	deadline := time.Now().Add(aCommandWait)
	for time.Now().Before(deadline) {
		taken, held, err := terminal.queue.Take(ctx)
		if err != nil {
			terminal.t.Fatalf("reading the queue failed: %v", err)
		}
		if held {
			return taken
		}
		time.Sleep(time.Millisecond)
	}
	terminal.t.Fatalf("%q never reached the queue in %s", line, aCommandWait)
	return channel.Queued{}
}

// finish marks one message done, which is what the loop does once it has
// answered.
func (terminal *aTerminal) finish(ctx context.Context, taken channel.Queued) {
	terminal.t.Helper()
	if err := terminal.queue.Done(ctx, taken.Sequence); err != nil {
		terminal.t.Fatalf("marking the message done failed: %v", err)
	}
}

// next reads the next envelope the program sent to the screen.
func (terminal *aTerminal) next() contract.SocketEnvelope {
	terminal.t.Helper()
	if err := terminal.screen.SetReadDeadline(time.Now().Add(aCommandWait)); err != nil {
		terminal.t.Fatalf("putting a deadline on the socket failed: %v", err)
	}
	line, err := terminal.lines.ReadBytes('\n')
	if err != nil {
		terminal.t.Fatalf("nothing reached the screen in %s: %v", aCommandWait, err)
	}
	envelope, err := contract.DecodeSocketEnvelope(line)
	if err != nil {
		terminal.t.Fatalf("the program sent the screen a line that is not a message: %v", err)
	}
	return envelope
}

// aTurnThatRewroteAFile writes the two events one turn leaves behind when it
// rewrites a file, and gives back the path of the file, so that "/undo" has
// something real to put back.
func (terminal *aTerminal) aTurnThatRewroteAFile() string {
	terminal.t.Helper()
	notes := filepath.Join(terminal.t.TempDir(), "notes.md")
	if err := os.WriteFile(notes, []byte("the words the agent wrote"), contract.DataFileMode); err != nil {
		terminal.t.Fatalf("writing the file the turn changed failed: %v", err)
	}

	terminal.append(contract.EventMessage, map[string]string{"text": "rewrite the notes"})
	terminal.append(contract.EventFileChange, contract.FileChangeBody{
		Path:          notes,
		Existed:       true,
		PriorContents: []byte("the words the user wrote"),
		Mode:          uint32(contract.DataFileMode),
	})
	return notes
}

// append writes one event into the log the commands read.
func (terminal *aTerminal) append(kind contract.EventKind, body any) {
	terminal.t.Helper()
	written, err := json.Marshal(body)
	if err != nil {
		terminal.t.Fatalf("writing the %s event body failed: %v", kind, err)
	}
	if _, err := terminal.store.Append(context.Background(), contract.Event{
		TaskID: "17", Kind: kind, Body: written,
	}); err != nil {
		terminal.t.Fatalf("appending the %s event failed: %v", kind, err)
	}
}

// startATerminal opens the socket, the queue, and the command registry, and
// attaches one screen to the socket, so that a test can type a line into a
// running agent and read what comes back.
func startATerminal(t *testing.T, ctx context.Context) *aTerminal {
	t.Helper()
	folder, err := os.MkdirTemp("", "coeus-socket")
	if err != nil {
		t.Fatalf("making a folder for the socket failed: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(folder) })

	queue, err := channel.OpenQueue(ctx, filepath.Join(t.TempDir(), "coeus.db"), contract.DefaultConfig().Caps.QueuedMessages)
	if err != nil {
		t.Fatalf("opening the queue failed: %v", err)
	}
	t.Cleanup(func() { _ = queue.Close() })

	stream := channel.NewStream(channel.StreamOptions{})
	socket, err := channel.Listen(channel.Options{
		Path:           filepath.Join(folder, "coeus.sock"),
		Stream:         stream,
		Queue:          queue,
		Secrets:        testkit.NewFakeSecrets(),
		Clock:          testkit.NewFakeClock(theStartOfTime),
		AnswerDeadline: contract.DefaultConfig().Caps.TimePerTurn,
	})
	if err != nil {
		t.Fatalf("listening on the socket failed: %v", err)
	}
	served := make(chan error, 1)
	go func() { served <- socket.Serve(ctx) }()
	t.Cleanup(func() {
		if err := socket.Close(); err != nil {
			t.Errorf("closing the socket failed: %v", err)
		}
		stream.Close()
		if err := <-served; err != nil {
			t.Errorf("serving the socket ended with %v", err)
		}
	})

	terminal := &aTerminal{t: t, socket: socket, queue: queue, store: testkit.NewFakeStore()}
	terminal.registry = theCoreCommands(t, terminal)
	terminal.attach(filepath.Join(folder, "coeus.sock"))
	return terminal
}

// attach opens one screen on the socket and waits until the program says it is
// reading the event stream, which is what a reply is written to.
func (terminal *aTerminal) attach(path string) {
	terminal.t.Helper()
	connection, err := net.Dial("unix", path)
	if err != nil {
		terminal.t.Fatalf("attaching a screen to the socket at %s failed: %v", path, err)
	}
	terminal.t.Cleanup(func() { _ = connection.Close() })
	terminal.screen, terminal.lines = connection, bufio.NewReader(connection)

	if err := contract.EncodeSocketEnvelope(connection, contract.SocketEnvelope{Type: contract.SocketAttach}); err != nil {
		terminal.t.Fatalf("asking for the event stream failed: %v", err)
	}
	deadline := time.Now().Add(aCommandWait)
	for time.Now().Before(deadline) {
		if terminal.socket.Attached() >= 1 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	terminal.t.Fatalf("no screen was reading the event stream after %s", aCommandWait)
}

// theCoreCommands registers the ten core commands over the fakes a fresh
// install would have behind them: one running job, a session that has cost
// something, the terminal itself as the one channel, and an event log.
func theCoreCommands(t *testing.T, terminal *aTerminal) *command.Registry {
	t.Helper()
	jobs := testkit.NewFakeJob(testkit.NewFakeClock(theStartOfTime))
	if _, err := jobs.Create(context.Background(), contract.NewJob{Ask: "run the anniversary campaign"}); err != nil {
		t.Fatalf("creating the job the status report lists failed: %v", err)
	}

	registry := command.NewRegistry()
	deps := command.Deps{
		Settings:     contract.DefaultConfig(),
		Store:        terminal.store,
		Jobs:         jobs,
		CurrentModel: func() string { return contract.LocalModelAlias },
		CostSoFar: func() contract.CostLine {
			return contract.CostLine{InputTokens: 1200, CachedInputTokens: 900, OutputTokens: 340}
		},
		PendingPreviews: func(context.Context) ([]contract.Preview, error) { return nil, nil },
		Channels:        func() []contract.Channel { return []contract.Channel{terminal.socket} },
	}
	for _, one := range command.New(registry, deps).All() {
		if err := registry.Register(one); err != nil {
			t.Fatalf("registering %s failed: %v", one.Name, err)
		}
	}
	return registry
}
