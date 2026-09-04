//go:build integration

package signal

import (
	"context"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// TestSignalEndToEndOnARealHome runs the whole of Signal against a real home
// folder: signal-cli started as a real child process in its own group, a
// stranger paired through the code they were sent, a photo downloaded into the
// inbox, a reply sent back, and everything shut down afterwards.
func TestSignalEndToEndOnARealHome(t *testing.T) {
	daemon := testkit.NewFakeSignalCLI()
	defer daemon.Close()
	photo := []byte("the bytes of a photograph")
	address := withAttachments(t, daemon, map[string][]byte{"attachment-1": photo})

	home := testkit.NewTempHome(t)
	clock := testkit.NewFakeClock(time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC))
	program, _ := writeProgram(t, "while true; do sleep 0.1; done")
	host, port := splitAddress(t, address)

	channel, err := NewChannel(ChannelOptions{
		Account: testAccount,
		Program: program,
		Host:    host,
		Port:    port,
		Home:    home,
		Clock:   clock,
		Secrets: testkit.NewFakeSecrets(),
	})
	if err != nil {
		t.Fatalf("cannot build the Signal channel: %v", err)
	}
	inbound := startChannel(t, channel)
	waitFor(t, "the event stream opens", func() bool { return channel.Connections() == 1 })

	child := runningChildOf(t, channel)
	pairThroughTheCode(t, channel, daemon)
	photoInTheInbox(t, channel, inbound, home, photo)
	replyGoesBack(t, channel, daemon)

	if err := channel.Close(); err != nil {
		t.Fatalf("closing the channel failed: %v", err)
	}
	waitFor(t, "signal-cli goes away", func() bool { return syscall.Kill(child, 0) != nil })
}

// runningChildOf checks that signal-cli was started for real, in a group of its
// own, and returns its process identifier.
func runningChildOf(t *testing.T, channel *Channel) int {
	t.Helper()
	child := channel.daemon.ProcessID()
	if child <= 0 {
		t.Fatalf("no signal-cli was started")
	}
	group, err := syscall.Getpgid(child)
	if err != nil || group != child {
		t.Fatalf("signal-cli is in group %d with error %v, want it to lead its own group %d", group, err, child)
	}
	return child
}

// pairThroughTheCode has a stranger write in, reads the code out of the message
// they were sent, checks the file it was kept in, and pairs them with the
// command the orchestrator registers.
func pairThroughTheCode(t *testing.T, channel *Channel, daemon *testkit.FakeSignalCLI) {
	t.Helper()
	const stranger = "+15125559999"
	daemon.PushMessage(stranger, "hello, can I talk to you")
	waitFor(t, "the stranger is offered a code", func() bool { return len(repliesTo(daemon, stranger)) > 0 })

	code := codeInside(t, repliesTo(daemon, stranger)[0])
	kept, err := os.ReadFile(channel.pairing.codesFile)
	if err != nil {
		t.Fatalf("cannot read the file the codes are kept in: %v", err)
	}
	if strings.Contains(string(kept), code) {
		t.Errorf("the code is written down in plain words, and a code is kept salted and hashed")
	}

	reply, err := PairCommand(channel.Pairing()).Run(context.Background(), code, contract.CommandContext{
		Channel: testkit.NewFakeChannel(contract.TerminalChannelName),
	})
	if err != nil {
		t.Fatalf("pairing the stranger with the code they were sent failed: %v", err)
	}
	if !strings.Contains(reply, stranger) {
		t.Errorf("the pairing command said %q, want it to name the sender it paired", reply)
	}
	if !channel.Pairing().IsApproved(stranger) {
		t.Fatalf("the stranger is still not approved after being paired")
	}
}

// photoInTheInbox sends a photo from the paired sender and checks it lands on
// the real filesystem with its path on the message.
func photoInTheInbox(t *testing.T, channel *Channel, inbound <-chan contract.Inbound, home contract.Home, photo []byte) {
	t.Helper()
	channel.route(context.Background(), Event{
		Sender:      "+15125559999",
		Text:        "here is the screen",
		Timestamp:   1700000000000,
		Attachments: []Attachment{{ID: "attachment-1", Filename: "screen.png", ContentType: "image/png"}},
	})

	message := takeMessage(t, inbound)
	if len(message.Attachments) != 1 {
		t.Fatalf("the message carries %d attachments, want the one photo", len(message.Attachments))
	}
	written, err := os.ReadFile(message.Attachments[0])
	if err != nil {
		t.Fatalf("cannot read the photo the message points at: %v", err)
	}
	if string(written) != string(photo) {
		t.Errorf("the photo in the inbox holds %q, want the bytes the daemon sent", written)
	}
	if !strings.HasPrefix(message.Attachments[0], home.InboxFolder()) {
		t.Errorf("the photo landed at %q, want it under the inbox %q", message.Attachments[0], home.InboxFolder())
	}
}

// replyGoesBack sends a reply and checks it reached the sender who last wrote.
func replyGoesBack(t *testing.T, channel *Channel, daemon *testkit.FakeSignalCLI) {
	t.Helper()
	if err := channel.Send(context.Background(), "Got it, thank you."); err != nil {
		t.Fatalf("sending a reply failed: %v", err)
	}
	sent := repliesTo(daemon, "+15125559999")
	if sent[len(sent)-1] != "Got it, thank you." {
		t.Errorf("the last message to the sender is %q, want the reply", sent[len(sent)-1])
	}
}

// codeInside pulls the eight-character code out of the pairing message.
func codeInside(t *testing.T, message string) string {
	t.Helper()
	_, after, found := strings.Cut(message, "code is ")
	if !found {
		t.Fatalf("the pairing message does not say what the code is: %q", message)
	}
	code, _, found := strings.Cut(after, ".")
	if !found {
		t.Fatalf("the pairing message does not end the code with a full stop: %q", message)
	}
	if _, valid := ReadPairingCode(code); !valid {
		t.Fatalf("the pairing message carries %q, which is not a pairing code", code)
	}
	return code
}
