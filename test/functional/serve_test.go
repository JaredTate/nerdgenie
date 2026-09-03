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
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	browserskill "github.com/JaredTate/coeus/internal/skill/browser"
	"github.com/JaredTate/coeus/internal/testkit"
)

// theReplyTheModelIsScriptedToGive is what the fake model server answers with,
// and what must come back over the socket unchanged.
const theReplyTheModelIsScriptedToGive = "Hello there, five words exactly."

func TestAMessageSentOverTheSocketComesBackAsTheModelsReply(t *testing.T) {
	agent := startTheAgent(t, testkit.Script{
		Name:          "local",
		ContextLength: 32768,
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
	agent := startTheAgent(t, testkit.Script{Name: "local", ContextLength: 32768})

	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketCommand, Text: "/readyz"})

	answer := screen.waitFor(t, contract.SocketReply, time.Second)
	if !strings.Contains(strings.ToLower(answer.Text), "ready") {
		t.Errorf("the answer to /readyz was %q, want a line saying the agent is ready", answer.Text)
	}
}

func TestASecondServeOnTheSameHomeRefusesToStart(t *testing.T) {
	agent := startTheAgent(t, testkit.Script{Name: "local", ContextLength: 32768})

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

// runningAgent is one child process of "coeus serve" with the home folder, the
// folder it may work in, and the scripted model server behind it.
type runningAgent struct {
	program  string
	home     contract.Home
	work     string
	model    *testkit.FakeProviderServer
	saidPath string
	wait     func() error
}

// startTheAgent starts the agent against a script that needs to know nothing
// about the folder the agent may work in.
func startTheAgent(t *testing.T, script testkit.Script) runningAgent {
	t.Helper()
	return startTheAgentWorkingIn(t, func(string) testkit.Script { return script })
}

// startTheAgentWorkingIn builds the binary, writes a home folder pointing at a
// scripted model server, starts "coeus serve" as its own process, and waits
// until it answers on its socket. The script is made from the folder the agent
// may work in, so that a step can name a file inside it. Everything it made is
// cleaned up when the test ends.
func startTheAgentWorkingIn(t *testing.T, makeScript func(work string) testkit.Script,
	andAlso ...func(home contract.Home, work string)) runningAgent {
	t.Helper()
	work := aWorkFolder(t)
	model := testkit.NewFakeProviderServer(makeScript(work))
	t.Cleanup(model.Close)

	program := buildTheBinary(t)
	home := aHomePointingAt(t, model.Address()+"/v1", work)
	for _, change := range andAlso {
		change(home, work)
	}

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
	// The child is waited for once and the answer kept, because a test that
	// asks how it ended and the cleanup that stops it are both waiting on it.
	ended := make(chan error, 1)
	waitFor := func() error {
		select {
		case err := <-stopped:
			ended <- err
			return err
		case err := <-ended:
			ended <- err
			return err
		}
	}
	t.Cleanup(func() {
		// The child is signalled by the exact process identifier the operating
		// system gave this run of it, so that nothing else on the machine can be
		// caught by the same stop.
		_ = started.Process.Signal(syscall.SIGTERM)
		select {
		case err := <-stopped:
			ended <- err
		case <-time.After(15 * time.Second):
			_ = started.Process.Kill()
		}
		t.Logf("what coeus serve said:\n%s", whatItSaid(saidPath))
	})

	if !waitingIsSkipped(andAlso) {
		waitForTheSocket(t, home, saidPath)
	}
	return runningAgent{program: program, home: home, work: work, model: model, saidPath: saidPath, wait: waitFor}
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

// aWorkFolder makes the one folder the agent may read and write in, which the
// configuration names as its sandbox root.
func aWorkFolder(t *testing.T) string {
	t.Helper()
	work := filepath.Join(t.TempDir(), "work")
	if err := os.MkdirAll(work, contract.HomeFolderMode); err != nil {
		t.Fatalf("making the work folder failed: %v", err)
	}
	return work
}

// aHomePointingAt makes a home folder whose one model alias is the scripted
// server, so that nothing in this test reaches the real machine's model.
func aHomePointingAt(t *testing.T, baseAddress string, work string) contract.Home {
	t.Helper()
	home := contract.NewHome(filepath.Join(t.TempDir(), ".coeus"))
	if err := os.MkdirAll(home.Root, contract.HomeFolderMode); err != nil {
		t.Fatalf("making the home folder failed: %v", err)
	}

	settings := fmt.Sprintf(`default_model = "local"
sandbox_roots = [%q]

[[models]]
name = "local"
provider = "openai"
base_address = %q
model_name = "local-coder"
context_length = 32768
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

// waitForStatusCarrying reads envelopes until a status arrives with something in
// the named field, which is how a test waits for the numbers the header draws
// rather than for the first status of any kind.
func (screen *attachedScreen) waitForStatusCarrying(t *testing.T, field string, bound time.Duration) contract.SocketEnvelope {
	t.Helper()
	return screen.waitUntil(t, bound, "a status carrying "+field, func(envelope contract.SocketEnvelope) bool {
		return envelope.Type == contract.SocketStatus && envelope.Fields[field] != ""
	})
}

// waitForReplySaying reads envelopes until a reply arrives with the words in it.
func (screen *attachedScreen) waitForReplySaying(t *testing.T, words string, bound time.Duration) contract.SocketEnvelope {
	t.Helper()
	return screen.waitUntil(t, bound, "a reply saying "+words, func(envelope contract.SocketEnvelope) bool {
		return envelope.Type == contract.SocketReply && strings.Contains(strings.ToLower(envelope.Text), strings.ToLower(words))
	})
}

// waitUntil reads envelopes until one of them is the one being waited for, and
// fails the test with what did arrive when none is.
func (screen *attachedScreen) waitUntil(t *testing.T, bound time.Duration, what string,
	itIsThis func(envelope contract.SocketEnvelope) bool) contract.SocketEnvelope {
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
		if itIsThis(envelope) {
			return envelope
		}
	}
	t.Fatalf("no %s arrived within %s; the agent sent %v (%v)", what, bound, seen, screen.lines.Err())
	return contract.SocketEnvelope{}
}

// waitForStatusWhere reads envelopes until a status arrives whose fields are the
// ones being waited for.
func (screen *attachedScreen) waitForStatusWhere(t *testing.T, bound time.Duration,
	itIsThis func(fields map[string]string) bool) contract.SocketEnvelope {
	t.Helper()
	return screen.waitUntil(t, bound, "a status with the fields wanted", func(envelope contract.SocketEnvelope) bool {
		return envelope.Type == contract.SocketStatus && itIsThis(envelope.Fields)
	})
}

// addSettingToTheHome writes one more line into the home folder's configuration,
// for a test that needs a setting the ordinary one does not carry.
func addSettingToTheHome(t *testing.T, home contract.Home, line string) {
	t.Helper()
	held, err := os.ReadFile(home.ConfigFile())
	if err != nil {
		t.Fatalf("reading the configuration to add a line failed: %v", err)
	}
	// The line goes above the first model block, because a key written after
	// one belongs to that block rather than to the file as a whole.
	written := strings.Replace(string(held), "\n[[models]]", "\n"+line+"\n\n[[models]]", 1)
	if err := os.WriteFile(home.ConfigFile(), []byte(written), contract.DataFileMode); err != nil {
		t.Fatalf("writing the configuration back failed: %v", err)
	}
}

// writeASkillOfTheirOwn puts one skill folder in the home, as a person who had
// already written that skill would have. It is built from the shipped one with
// its words changed, so that the store can read it and the test is about whose
// copy survives rather than about the file format.
func writeASkillOfTheirOwn(t *testing.T, home contract.Home, name string, says string) {
	t.Helper()
	folder := home.SkillFolder(name)
	if err := os.MkdirAll(folder, contract.HomeFolderMode); err != nil {
		t.Fatalf("making the skill folder failed: %v", err)
	}
	for called, held := range browserskill.ShippedQASkill() {
		written := strings.Replace(string(held), "Walks an app step by step", says, 1)
		if err := os.WriteFile(filepath.Join(folder, called), []byte(written), contract.DataFileMode); err != nil {
			t.Fatalf("writing %s of the skill failed: %v", called, err)
		}
	}
}

// doNotWaitForTheSocket says a test expects the agent to stop rather than serve,
// so the starter must not wait thirty seconds for a socket that never opens. It
// is passed as one of the changes to the home, and changes nothing itself.
func doNotWaitForTheSocket(contract.Home, string) {}

// waitingIsSkipped says whether one of the changes was doNotWaitForTheSocket.
func waitingIsSkipped(andAlso []func(home contract.Home, work string)) bool {
	for _, change := range andAlso {
		if reflect.ValueOf(change).Pointer() == reflect.ValueOf(doNotWaitForTheSocket).Pointer() {
			return true
		}
	}
	return false
}
