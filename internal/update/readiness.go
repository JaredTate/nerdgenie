// Blocking an update on a health check after the restart follows the post-
// restart probe in OpenClaw at ~/Code/openclaw/src/cli/daemon-cli/, which asks
// the daemon it just restarted whether it is really up rather than trusting the
// service manager's word for it. What is different here is that there is no port
// to probe: the readiness check is a slash command the agent answers on its own
// local socket, so the answer proves the whole way in and out is working.

package update

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// ReadyCommand is the slash command "nerdgenie serve" answers while it is running,
// which is the /readyz check of design section 11.
const ReadyCommand = "/readyz"

// The bounds on one readiness question.
const (
	// dialWait is how long opening the socket may take.
	dialWait = 2 * time.Second
	// answerWait is how long the agent has to answer one question before the
	// updater gives up on this attempt and asks again.
	answerWait = 5 * time.Second
	// maxReadyLines is how many messages the agent may send before the answer,
	// because a screen attaching sees whatever the loop is publishing.
	maxReadyLines = 64
	// maxReadyBytes is the most one answer may be.
	maxReadyBytes = 1 << 20
)

// askIfReady attaches to the agent's local socket, asks the readiness command,
// and returns nothing at all when the agent answers.
//
// Any reply counts as an answer. The words in it are not compared, because what
// readiness means is that a message went in through the socket, through the
// queue, and came back out through the router, and a version of Coeus that says
// it differently is still a version that is up.
func askIfReady(ctx context.Context, home contract.Home) error {
	dialer := net.Dialer{Timeout: dialWait}
	connection, err := dialer.DialContext(ctx, "unix", home.SocketFile())
	if err != nil {
		return fmt.Errorf("the agent is not answering on %s: %w", home.SocketFile(), err)
	}
	defer func() { _ = connection.Close() }()

	if err := connection.SetDeadline(time.Now().Add(answerWait)); err != nil {
		return fmt.Errorf("the socket %s would not take a deadline: %w", home.SocketFile(), err)
	}

	// A write that failed is not reported until the reading has been tried,
	// because an agent that answers and hangs up straight away breaks the second
	// write, and what it said is the better answer. The write is only the story
	// when the agent said nothing at all.
	sent := askTheQuestion(connection, home)
	heard, answered := readTheAnswer(bufio.NewReader(io.LimitReader(connection, maxReadyBytes)))
	if !heard && sent != nil {
		return sent
	}
	return answered
}

// askTheQuestion attaches to the agent as a screen does and sends the readiness
// command, and hands back the first trouble it met.
func askTheQuestion(connection net.Conn, home contract.Home) error {
	if err := contract.EncodeSocketEnvelope(connection, contract.SocketEnvelope{Type: contract.SocketAttach}); err != nil {
		return fmt.Errorf("the readiness check could not attach to %s: %w", home.SocketFile(), err)
	}
	if err := contract.EncodeSocketEnvelope(connection, contract.SocketEnvelope{
		Type: contract.SocketCommand, Text: ReadyCommand,
	}); err != nil {
		return fmt.Errorf("the readiness check could not be sent to %s: %w", home.SocketFile(), err)
	}
	return nil
}

// readTheAnswer reads what the agent sent back until it finds a reply, and gives
// up after a bounded number of lines so that a talkative agent cannot hold the
// update open. It also says whether the agent sent anything at all, because an
// agent that said nothing is one the caller should report the sending trouble
// for instead.
func readTheAnswer(lines *bufio.Reader) (bool, error) {
	heard := false
	for range maxReadyLines {
		line, err := lines.ReadBytes('\n')
		if err != nil && len(line) == 0 {
			return heard, fmt.Errorf("the agent stopped answering the readiness check part way through: %w", err)
		}
		envelope, err := contract.DecodeSocketEnvelope(line)
		if err != nil {
			if errors.Is(err, contract.ErrEmptySocketLine) {
				continue
			}
			return true, fmt.Errorf("the agent answered the readiness check with something that is not a socket message: %w", err)
		}
		heard = true
		switch envelope.Type {
		case contract.SocketReply, contract.SocketStatus:
			return true, nil
		case contract.SocketError:
			return true, fmt.Errorf("the agent answered the readiness check with an error: %s", envelope.Text)
		}
	}
	return heard, fmt.Errorf("the agent sent %d messages without answering the readiness check, so it is not serving properly", maxReadyLines)
}
