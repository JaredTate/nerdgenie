package tui

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// listenOnATempSocket starts a listener on a socket under a temporary home and
// hands back the home, the one link the screen makes to it, and the listener
// itself, so that a test can take the program away.
func listenOnATempSocket(t *testing.T) (contract.Home, chan net.Conn, net.Listener) {
	t.Helper()
	home := testkit.NewTempHome(t)
	if err := os.MkdirAll(home.RunFolder(), contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot make the run folder: %v", err)
	}
	listener, err := net.Listen("unix", home.SocketFile())
	if err != nil {
		t.Fatalf("cannot listen on the socket: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	accepted := make(chan net.Conn, 1)
	go func() {
		link, acceptErr := listener.Accept()
		if acceptErr == nil {
			accepted <- link
		}
	}()
	return home, accepted, listener
}

// acceptOne waits for the screen to reach the listener.
func acceptOne(t *testing.T, accepted chan net.Conn) net.Conn {
	t.Helper()
	select {
	case link := <-accepted:
		t.Cleanup(func() { _ = link.Close() })
		return link
	case <-time.After(waitingLimit):
		t.Fatal("nothing reached the listener, and the dialer should have connected by now")
		return nil
	}
}

func TestTheUnixDialerReachesTheSocketAndCarriesEnvelopes(t *testing.T) {
	home, accepted, _ := listenOnATempSocket(t)
	dialer := NewUnixDialer(home)

	connection, err := dialer.Dial(context.Background())
	if err != nil {
		t.Fatalf("cannot reach the socket the program listens on: %v", err)
	}
	defer func() { _ = connection.Close() }()
	link := acceptOne(t, accepted)

	if err := connection.Send(contract.SocketEnvelope{Type: contract.SocketAttach}); err != nil {
		t.Fatalf("cannot send on the socket: %v", err)
	}
	line, err := bufio.NewReader(link).ReadBytes('\n')
	if err != nil {
		t.Fatalf("nothing arrived on the program's side: %v", err)
	}
	arrived, err := contract.DecodeSocketEnvelope(line)
	if err != nil || arrived.Type != contract.SocketAttach {
		t.Fatalf("the program read %q, and the screen sent an attach", line)
	}

	if err := contract.EncodeSocketEnvelope(link, contract.SocketEnvelope{Type: contract.SocketReply, Text: "hello"}); err != nil {
		t.Fatalf("cannot answer from the program's side: %v", err)
	}
	answer, err := connection.Receive()
	if err != nil || answer.Text != "hello" {
		t.Fatalf("the screen read %+v and %v, and the program said hello", answer, err)
	}
}

func TestTheDialerSaysSoWhenNothingIsListening(t *testing.T) {
	home := contract.NewHome(filepath.Join(t.TempDir(), "nowhere"))
	if _, err := NewUnixDialer(home).Dial(context.Background()); err == nil {
		t.Fatal("dialling a socket nothing listens on said nothing went wrong")
	}
}

func TestALineTooLongIsDroppedWithAnErrorRatherThanRead(t *testing.T) {
	huge := bytes.Repeat([]byte("a"), maxSocketLineBytes+10)
	reader := bufio.NewReader(bytes.NewReader(append(append(huge, '\n'), []byte("{\"type\":\"reply\"}\n")...)))

	if _, err := readCappedLine(reader, maxSocketLineBytes); !errors.Is(err, errLineTooLong) {
		t.Fatalf("reading a line past the cap gave %v, and it must say the line was too long", err)
	}
	next, err := readCappedLine(reader, maxSocketLineBytes)
	if err != nil {
		t.Fatalf("the line after the long one could not be read: %v", err)
	}
	if !strings.Contains(string(next), "reply") {
		t.Errorf("the line after the long one reads %q, and the rest of the long line should have been thrown away", next)
	}
}

func TestALineThatIsNotAMessageBecomesAnErrorCardRatherThanACrash(t *testing.T) {
	home, accepted, _ := listenOnATempSocket(t)
	connection, err := NewUnixDialer(home).Dial(context.Background())
	if err != nil {
		t.Fatalf("cannot reach the socket: %v", err)
	}
	defer func() { _ = connection.Close() }()
	link := acceptOne(t, accepted)

	if _, err := link.Write([]byte("this is not JSON at all\n")); err != nil {
		t.Fatalf("cannot write the broken line: %v", err)
	}
	answer, err := connection.Receive()
	if err != nil {
		t.Fatalf("a broken line closed the link, and it should become an error instead: %v", err)
	}
	if answer.Type != contract.SocketError {
		t.Errorf("a broken line came back as %+v, and it should be an error the screen can show", answer)
	}
}

func FuzzReadCappedLine(f *testing.F) {
	f.Add([]byte("{\"type\":\"reply\",\"text\":\"hello\"}\n"))
	f.Add([]byte("\n\n\n"))
	f.Add(append(bytes.Repeat([]byte("x"), 200), '\n'))
	f.Fuzz(func(t *testing.T, given []byte) {
		reader := bufio.NewReader(bytes.NewReader(given))
		for range 100 {
			line, err := readCappedLine(reader, 128)
			if err != nil {
				return
			}
			if len(line) > 128 {
				t.Fatalf("a line of %d bytes came back and the cap is 128", len(line))
			}
		}
	})
}
