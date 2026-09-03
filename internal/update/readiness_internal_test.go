package update

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

// anAgentAnswering listens on a home's socket and hands every caller to the
// answer function, which is what stands in for a running agent. The home is
// under a short path, because a Unix socket path is far shorter than a file path
// may be.
func anAgentAnswering(t *testing.T, answer func(connection net.Conn)) contract.Home {
	t.Helper()
	folder, err := os.MkdirTemp("", "coeus-ready")
	if err != nil {
		t.Fatalf("making a folder for the socket failed: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(folder) })

	home := contract.NewHome(filepath.Join(folder, ".coeus"))
	if err := os.MkdirAll(home.RunFolder(), contract.HomeFolderMode); err != nil {
		t.Fatalf("making the run folder failed: %v", err)
	}
	if answer == nil {
		return home
	}

	listener, err := net.Listen("unix", home.SocketFile())
	if err != nil {
		t.Fatalf("listening on %s failed: %v", home.SocketFile(), err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			answer(connection)
			_ = connection.Close()
		}
	}()
	return home
}

func TestAnAgentThatRepliesIsReady(t *testing.T) {
	home := anAgentAnswering(t, func(connection net.Conn) {
		_ = contract.EncodeSocketEnvelope(connection, contract.SocketEnvelope{Type: contract.SocketReply, Text: "ready"})
	})

	if err := askIfReady(context.Background(), home); err != nil {
		t.Errorf("an agent that answered the readiness check was called not ready: %v", err)
	}
}

func TestAnAgentThatAnswersWithAnErrorIsNotReady(t *testing.T) {
	home := anAgentAnswering(t, func(connection net.Conn) {
		_ = contract.EncodeSocketEnvelope(connection, contract.SocketEnvelope{
			Type: contract.SocketError, Text: "the queue is full",
		})
	})

	err := askIfReady(context.Background(), home)

	if err == nil {
		t.Fatalf("an agent that answered with an error was called ready")
	}
	if !strings.Contains(err.Error(), "the queue is full") {
		t.Errorf("the report does not say what the agent answered: %v", err)
	}
}

func TestAnAgentThatTalksNonsenseIsNotReady(t *testing.T) {
	home := anAgentAnswering(t, func(connection net.Conn) {
		_, _ = connection.Write([]byte("this is not a socket message\n"))
	})

	if err := askIfReady(context.Background(), home); err == nil {
		t.Errorf("an agent that answered with something that is not a message was called ready")
	}
}

func TestAnAgentThatNeverAnswersIsNotReady(t *testing.T) {
	home := anAgentAnswering(t, func(connection net.Conn) {
		for range maxReadyLines + 1 {
			if err := contract.EncodeSocketEnvelope(connection, contract.SocketEnvelope{
				Type: contract.SocketDelta, Text: "still writing",
			}); err != nil {
				return
			}
		}
	})

	err := askIfReady(context.Background(), home)

	if err == nil {
		t.Fatalf("an agent that never answered was called ready")
	}
	if !strings.Contains(err.Error(), "without answering") {
		t.Errorf("the report does not say that the answer never came: %v", err)
	}
}

func TestAnAgentThatHangsUpAtOnceIsNotReady(t *testing.T) {
	home := anAgentAnswering(t, func(connection net.Conn) {})

	if err := askIfReady(context.Background(), home); err == nil {
		t.Errorf("an agent that hung up without a word was called ready")
	}
}

func TestASocketNobodyIsListeningOnIsNotReady(t *testing.T) {
	home := anAgentAnswering(t, nil)

	err := askIfReady(context.Background(), home)

	if err == nil {
		t.Fatalf("a home with no agent running was called ready")
	}
	if !strings.Contains(err.Error(), home.SocketFile()) {
		t.Errorf("the report does not name the socket it tried: %v", err)
	}
}
