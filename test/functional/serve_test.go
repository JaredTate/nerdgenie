// The whole-program test: "coeus serve" is started as its own process against a
// temporary home folder and a scripted model server, a screen attaches over the
// same local socket the terminal uses, sends a message, and reads the reply.
//
// The binary is built and run rather than called, because cmd/coeus is package
// main and no test can import it. The child is stopped by its exact process
// identifier, never by anything matching a name.
package functional

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// theReplyTheModelIsScriptedToGive is what the fake model server answers with,
// and what must come back over the socket unchanged.
const theReplyTheModelIsScriptedToGive = "Hello there, five words exactly."

func TestAMessageSentOverTheSocketComesBackAsTheModelsReply(t *testing.T) {
	agent := startTheAgent(t, testkit.Script{
		Name:          "local",
		ContextLength: 8192,
		Steps: []testkit.Step{{
			Expect: []string{"Say hello in five words.", "You are the reasoning engine inside Coeus"},
			Text:   theReplyTheModelIsScriptedToGive,
			Finish: contract.FinishEnd,
			Usage:  contract.Usage{InputTokens: 300, OutputTokens: 8},
		}},
	})

	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: "Say hello in five words."})

	reply := screen.waitFor(t, contract.SocketReply, 30*time.Second)
	if reply.Text != theReplyTheModelIsScriptedToGive {
		t.Errorf("the reply was %q, want the one the script wrote", reply.Text)
	}
}

func TestTheAgentAnswersReadyzOnTheSocketWithinASecond(t *testing.T) {
	agent := startTheAgent(t, testkit.Script{Name: "local", ContextLength: 8192})

	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketCommand, Text: "/readyz"})

	answer := screen.waitFor(t, contract.SocketReply, time.Second)
	if !strings.Contains(strings.ToLower(answer.Text), "ready") {
		t.Errorf("the answer to /readyz was %q, want a line saying the agent is ready", answer.Text)
	}
}

func TestASecondServeOnTheSameHomeRefusesToStart(t *testing.T) {
	agent := startTheAgent(t, testkit.Script{Name: "local", ContextLength: 8192})

	second := exec.Command(agent.program, "serve")
	second.Env = append(os.Environ(), "COEUS_HOME="+agent.home.Root)
	said, err := second.CombinedOutput()

	if err == nil {
		t.Fatalf("a second coeus serve on the same home folder started anyway:\n%s", said)
	}
	if !strings.Contains(string(said), agent.home.LockFile()) {
		t.Errorf("the refusal does not name the lock file that stopped it:\n%s", said)
	}
}

// runningAgent is one child process of "coeus serve" with the home folder and
// the scripted model server behind it.
type runningAgent struct {
	program string
	home    contract.Home
}

// startTheAgent builds the binary, writes a home folder pointing at a scripted
// model server, starts "coeus serve" as its own process, and waits until it
// answers on its socket. Everything it made is cleaned up when the test ends.
func startTheAgent(t *testing.T, script testkit.Script) runningAgent {
	t.Helper()
	model := testkit.NewFakeProviderServer(script)
	t.Cleanup(model.Close)

	program := buildTheBinary(t)
	home := aHomePointingAt(t, model.Address()+"/v1")

	saidPath := filepath.Join(t.TempDir(), "coeus-serve.log")
	said, err := os.Create(saidPath)
	if err != nil {
		t.Fatalf("making the file for what coeus serve says failed: %v", err)
	}
	t.Cleanup(func() { _ = said.Close() })

	started := exec.Command(program, "serve")
	started.Env = append(os.Environ(), "COEUS_HOME="+home.Root)
	started.Stdout = said
	started.Stderr = said
	if err := started.Start(); err != nil {
		t.Fatalf("starting coeus serve failed: %v", err)
	}

	stopped := make(chan error, 1)
	go func() { stopped <- started.Wait() }()
	t.Cleanup(func() {
		// The child is signalled by the exact process identifier the operating
		// system gave this run of it, so that nothing else on the machine can be
		// caught by the same stop.
		_ = started.Process.Signal(syscall.SIGTERM)
		select {
		case <-stopped:
		case <-time.After(15 * time.Second):
			_ = started.Process.Kill()
		}
		t.Logf("what coeus serve said:\n%s", whatItSaid(saidPath))
	})

	waitForTheSocket(t, home, saidPath)
	return runningAgent{program: program, home: home}
}

// buildTheBinary compiles cmd/coeus into a folder of this test's own, so that
// the test drives the real program rather than a copy of its parts.
func buildTheBinary(t *testing.T) string {
	t.Helper()
	program := filepath.Join(t.TempDir(), "coeus")
	build := exec.Command("go", "build", "-o", program, "./cmd/coeus")
	build.Dir = repositoryRoot(t)
	if said, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building the coeus binary failed: %v\n%s", err, said)
	}
	return program
}

// repositoryRoot is the folder holding go.mod, which is two above this package.
func repositoryRoot(t *testing.T) string {
	t.Helper()
	here, err := os.Getwd()
	if err != nil {
		t.Fatalf("finding the working directory failed: %v", err)
	}
	root := filepath.Dir(filepath.Dir(here))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("the repository root %s holds no go.mod: %v", root, err)
	}
	return root
}

// aHomePointingAt makes a home folder whose one model alias is the scripted
// server, so that nothing in this test reaches the real machine's model.
func aHomePointingAt(t *testing.T, baseAddress string) contract.Home {
	t.Helper()
	home := contract.NewHome(filepath.Join(t.TempDir(), ".coeus"))
	if err := os.MkdirAll(home.Root, contract.HomeFolderMode); err != nil {
		t.Fatalf("making the home folder failed: %v", err)
	}
	work := filepath.Join(t.TempDir(), "work")
	if err := os.MkdirAll(work, contract.HomeFolderMode); err != nil {
		t.Fatalf("making the work folder failed: %v", err)
	}

	settings := fmt.Sprintf(`default_model = "local"
sandbox_roots = [%q]

[[models]]
name = "local"
provider = "openai"
base_address = %q
model_name = "local-coder"
context_length = 8192
`, work, baseAddress)
	if err := os.WriteFile(home.ConfigFile(), []byte(settings), contract.DataFileMode); err != nil {
		t.Fatalf("writing the configuration failed: %v", err)
	}
	return home
}

// waitForTheSocket waits until the agent answers on its socket, and fails with
// whatever the agent printed when it never does.
func waitForTheSocket(t *testing.T, home contract.Home, saidPath string) {
	t.Helper()
	giveUp := time.Now().Add(30 * time.Second)
	for time.Now().Before(giveUp) {
		connection, err := net.Dial("unix", home.SocketFile())
		if err == nil {
			_ = connection.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("coeus serve never opened its socket at %s; it said:\n%s", home.SocketFile(), whatItSaid(saidPath))
}

// whatItSaid reads back everything the child process printed, for a failure
// message that says why it never came up.
func whatItSaid(path string) string {
	printed, err := os.ReadFile(path)
	if err != nil {
		return "nothing could be read back: " + err.Error()
	}
	return string(printed)
}

// attachedScreen is one connection to the agent's socket, reading one JSON
// envelope per line exactly as the terminal screen does.
type attachedScreen struct {
	connection net.Conn
	lines      *bufio.Scanner
}

// attach opens a connection to the agent and asks for the event stream.
func (agent runningAgent) attach(t *testing.T) *attachedScreen {
	t.Helper()
	connection, err := net.Dial("unix", agent.home.SocketFile())
	if err != nil {
		t.Fatalf("attaching to the agent failed: %v", err)
	}
	t.Cleanup(func() { _ = connection.Close() })

	screen := &attachedScreen{connection: connection, lines: bufio.NewScanner(connection)}
	screen.lines.Buffer(make([]byte, 4096), 1<<20)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketAttach})
	return screen
}

// send writes one envelope to the agent.
func (screen *attachedScreen) send(t *testing.T, envelope contract.SocketEnvelope) {
	t.Helper()
	if err := screen.connection.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("setting the write deadline failed: %v", err)
	}
	if err := contract.EncodeSocketEnvelope(screen.connection, envelope); err != nil {
		t.Fatalf("sending %s failed: %v", envelope.Type, err)
	}
}

// waitFor reads envelopes until one of the wanted kind arrives, and fails the
// test when nothing of that kind arrives inside the bound.
func (screen *attachedScreen) waitFor(t *testing.T, wanted contract.SocketMessageType, bound time.Duration) contract.SocketEnvelope {
	t.Helper()
	if err := screen.connection.SetReadDeadline(time.Now().Add(bound)); err != nil {
		t.Fatalf("setting the read deadline failed: %v", err)
	}
	seen := []string{}
	for screen.lines.Scan() {
		envelope := contract.SocketEnvelope{}
		if err := json.Unmarshal(screen.lines.Bytes(), &envelope); err != nil {
			t.Fatalf("the agent sent a line that is not a JSON object: %q", screen.lines.Text())
		}
		seen = append(seen, string(envelope.Type))
		if envelope.Type == contract.SocketError {
			t.Fatalf("the agent answered with an error: %s %s", envelope.Text, envelope.Reason)
		}
		if envelope.Type == wanted {
			return envelope
		}
	}
	t.Fatalf("no %s arrived within %s; the agent sent %v (%v)", wanted, bound, seen, screen.lines.Err())
	return contract.SocketEnvelope{}
}
