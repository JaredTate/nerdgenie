//go:build live

// The live tier of the functional suite: a real "coeus serve" driven over the
// same socket the terminal uses, against the three real models this machine
// has, rather than against a scripted one.
//
// Nothing here ever skips. A daemon that is down, a program that is missing,
// and a program that is not signed in are all failures, because the whole point
// of the live suite is to find out that one of them has stopped working.
//
// The three models are the ones CLAUDE.md describes: the Qwen the llama-server
// daemon serves on this machine, Opus through `claude -p` on the user's
// subscription, and GPT through `codex exec` on the same footing. There are no
// API keys on this machine and none are wanted, so both cloud models are
// reached by running the vendor's own program once per call.
package functional

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// The three models the live suite calls, written as CLAUDE.md writes them: the
// address the local daemon answers on, the name each model is asked for by, and
// the window each one holds.
const (
	liveLocalAddress     = "http://127.0.0.1:19091/v1"
	liveLocalModelName   = "local-coder"
	liveLocalWindow      = 262144
	liveOpusModelName    = "claude-opus-4-8"
	liveCodexModelName   = "gpt-5.5"
	liveSubscriptionSize = 200000
)

// The two bounds a live reply works inside. The local model runs on one
// graphics card and is the slower of the two kinds, so it is given twice as
// long as a subscription program.
const (
	liveLocalBound        = 10 * time.Minute
	liveSubscriptionBound = 5 * time.Minute
)

// liveModel is one of the three models a live test runs against: the alias the
// configuration calls it by, the vendor program behind it when it is a
// subscription, the window it holds, and how long a whole reply may take before
// the test gives up on it.
type liveModel struct {
	alias   string
	program string
	window  int
	bound   time.Duration
}

// theLocalModel is the Qwen the llama-server daemon on this machine serves,
// which is the alias a fresh install ships with.
func theLocalModel() liveModel {
	return liveModel{alias: contract.LocalModelAlias, window: liveLocalWindow, bound: liveLocalBound}
}

// theClaudeModel is Opus on the user's subscription, through the claude program.
func theClaudeModel() liveModel {
	return liveModel{
		alias:   "claude",
		program: contract.ClaudeProgram,
		window:  liveSubscriptionSize,
		bound:   liveSubscriptionBound,
	}
}

// theCodexModel is GPT on the user's subscription, through the codex program.
func theCodexModel() liveModel {
	return liveModel{
		alias:   "codex",
		program: contract.CodexProgram,
		window:  liveSubscriptionSize,
		bound:   liveSubscriptionBound,
	}
}

// liveConfiguration is the home folder's config.toml for a live run: the three
// real models exactly as CLAUDE.md describes them, one of them as the default,
// and the folder the agent may work in as its only sandbox root.
func liveConfiguration(defaultModel string, work string) string {
	return fmt.Sprintf(`default_model = %q
sandbox_roots = [%q]

[[models]]
name = %q
provider = "openai"
base_address = %q
model_name = %q
context_length = %d

[[models]]
name = "claude"
provider = "cli"
program = %q
model_name = %q
context_length = %d

[[models]]
name = "codex"
provider = "cli"
program = %q
model_name = %q
context_length = %d
`,
		defaultModel, work,
		contract.LocalModelAlias, liveLocalAddress, liveLocalModelName, liveLocalWindow,
		contract.ClaudeProgram, liveOpusModelName, liveSubscriptionSize,
		contract.CodexProgram, liveCodexModelName, liveSubscriptionSize)
}

// startTheLiveAgent starts a real "coeus serve" against a home naming the three
// real models, with this one as the default, and waits until it answers on its
// socket. Anything a test needs done to the home before the agent starts, such
// as seeding a record, is passed after the model.
//
// The starter it borrows always makes a scripted model server as well. Nothing
// in a live run ever reaches that server, because the configuration written
// over the home here names the three real models in its place.
func startTheLiveAgent(t *testing.T, model liveModel,
	alsoBeforeItStarts ...func(home contract.Home, work string)) runningAgent {
	t.Helper()
	requireTheModelIsThere(t, model)
	changes := append([]func(home contract.Home, work string){useTheRealModels(t, model)}, alsoBeforeItStarts...)
	return startTheAgentWorkingIn(t, func(string) testkit.Script {
		return testkit.Script{Name: model.alias, ContextLength: model.window}
	}, changes...)
}

// livePersona is the SOUL.md a live home is given. It says the one thing the
// shipped instruction text says only in passing and no real model does on its
// own for a small task: keep the record as you go, and prove each done line with
// the result behind it. Without this line all three models write the file, say
// they are finished, and leave the task waiting with an empty done list, because
// a reply with nothing behind it is read as a question.
const livePersona = "You are Coeus. You do the work with your tools and you keep the task record as you go.\n" +
	"Before you answer that the work is finished, call the task tool to write the done list, " +
	"with each line marked done and naming the id of the result that proves it, such as r1."

// useTheRealModels writes the live configuration and the persona over what the
// starter wrote, before the agent reads either.
func useTheRealModels(t *testing.T, model liveModel) func(home contract.Home, work string) {
	return func(home contract.Home, work string) {
		t.Helper()
		settings := liveConfiguration(model.alias, work)
		if err := os.WriteFile(home.ConfigFile(), []byte(settings), contract.DataFileMode); err != nil {
			t.Fatalf("writing the live configuration to %s failed: %v", home.ConfigFile(), err)
		}
		if err := os.MkdirAll(home.PersonaFolder(), contract.HomeFolderMode); err != nil {
			t.Fatalf("making the persona folder %s failed: %v", home.PersonaFolder(), err)
		}
		if err := os.WriteFile(home.SoulFile(), []byte(livePersona), contract.DataFileMode); err != nil {
			t.Fatalf("writing the persona to %s failed: %v", home.SoulFile(), err)
		}
	}
}

// requireTheModelIsThere fails when the thing behind the model is not running:
// the daemon for the local model, the vendor program for a subscription one. A
// live test never skips, so this says what to do rather than standing down.
func requireTheModelIsThere(t *testing.T, model liveModel) {
	t.Helper()
	if model.program == "" {
		requireTheLocalDaemonIsUp(t)
		return
	}
	if _, err := exec.LookPath(model.program); err != nil {
		t.Fatalf("the program %s is not on the path, so install it, sign in, and put its folder and node's folder on PATH: %v",
			model.program, err)
	}
}

// requireTheLocalDaemonIsUp fails when the llama-server daemon is not answering,
// and says how to start it.
func requireTheLocalDaemonIsUp(t *testing.T) {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	answer, err := client.Get(strings.TrimSuffix(liveLocalAddress, "/v1") + "/health")
	if err != nil {
		t.Fatalf("the local model daemon at %s is not answering, so start it with ~/llm/igo.sh and try again: %v",
			liveLocalAddress, err)
	}
	defer func() { _ = answer.Body.Close() }()
	if answer.StatusCode != http.StatusOK {
		t.Fatalf("the local model daemon at %s answered its health check with %d, want 200",
			liveLocalAddress, answer.StatusCode)
	}
}

// liveRun is what one message through the live agent cost: the reply that came
// back, how long it took, the numbers the status carried, and the line the
// status command prints about what the session has spent.
type liveRun struct {
	reply    contract.SocketEnvelope
	took     time.Duration
	status   map[string]string
	costLine string
}

// report writes the timing and the numbers into the test log, so that
// docs/PROGRESS.md can carry what each model costs.
func (run liveRun) report(t *testing.T, model liveModel, what string) {
	t.Helper()
	t.Logf("%s: %s took %s: %s tokens in, %s out, %s dollars, and the last call held %s of the %s the model can hold",
		model.alias, what, run.took.Round(time.Second),
		orNothingYet(run.status[contract.StatusFieldTokensIn]),
		orNothingYet(run.status[contract.StatusFieldTokensOut]),
		orNothingYet(run.status[contract.StatusFieldCost]),
		orNothingYet(run.status[contract.StatusFieldContextTokens]),
		orNothingYet(run.status[contract.StatusFieldContextWindow]))
	t.Logf("%s: what the agent says the session cost: %s", model.alias, run.costLine)
}

// orNothingYet is what a status field says when the agent never filled it in,
// so that a log line about a missing number reads as a sentence.
func orNothingYet(value string) string {
	if strings.TrimSpace(value) == "" {
		return "no"
	}
	return value
}

// sendAndWaitForTheReply sends one message to the agent and returns when the
// reply arrives, with the numbers of the run. A reply that has not arrived
// inside the model's bound fails the test with everything the agent printed.
//
// The token counts are asked for again once the reply is in, because the status
// carrying them is sent as the turn ends and can arrive after the reply it
// belongs to.
func (agent runningAgent) sendAndWaitForTheReply(t *testing.T, screen *attachedScreen,
	text string, bound time.Duration) liveRun {
	t.Helper()
	numbers := map[string]string{}
	began := time.Now()
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: text})
	reply := agent.readUntilTheReply(t, screen, numbers, bound, "the reply to the message")
	run := liveRun{reply: reply, took: time.Since(began), status: numbers}

	screen.send(t, contract.SocketEnvelope{Type: contract.SocketCommand, Text: "status"})
	said := agent.readUntilTheReply(t, screen, numbers, aCommandIsAnsweredWithin, "the answer to the status command")
	run.costLine = costSoFarIn(said.Text)
	return run
}

// readUntilTheReply reads envelopes until a reply arrives, saying yes to every
// preview on the way and keeping the numbers off every status, because both can
// arrive in the middle of anything else.
func (agent runningAgent) readUntilTheReply(t *testing.T, screen *attachedScreen,
	numbers map[string]string, bound time.Duration, waitingFor string) contract.SocketEnvelope {
	t.Helper()
	if err := screen.connection.SetReadDeadline(time.Now().Add(bound)); err != nil {
		t.Fatalf("setting the read deadline failed: %v", err)
	}
	for screen.lines.Scan() {
		envelope := contract.SocketEnvelope{}
		if err := json.Unmarshal(screen.lines.Bytes(), &envelope); err != nil {
			t.Fatalf("the agent sent a line that is not a JSON object: %q", screen.lines.Text())
		}
		switch envelope.Type {
		case contract.SocketPreview:
			t.Logf("approving the preview %q: %s", envelope.Title, oneLine(envelope.Text))
			screen.send(t, contract.SocketEnvelope{Type: contract.SocketApprove, ID: envelope.ID})
		case contract.SocketStatus:
			keepTheNumbers(numbers, envelope.Fields)
		case contract.SocketAsk:
			t.Fatalf("the agent asked the user something instead of doing the work: %s", envelope.Text)
		case contract.SocketError:
			t.Fatalf("the agent answered with an error: %s %s", envelope.Text, envelope.Reason)
		case contract.SocketReply:
			return envelope
		}
	}
	t.Fatalf("%s never arrived within %s; the agent said:\n%s", waitingFor, bound, whatItSaid(agent.saidPath))
	return contract.SocketEnvelope{}
}

// costSoFarIn is the one line of the status command that says what the session
// has spent, which is the number docs/PROGRESS.md carries.
func costSoFarIn(said string) string {
	for _, line := range strings.Split(said, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "cost so far:") {
			return strings.TrimSpace(line)
		}
	}
	return "the status command said nothing about the cost:\n" + said
}

// keepTheNumbers copies the fields of one status over what earlier ones said,
// leaving an earlier number in place where the newest status carries none.
func keepTheNumbers(kept map[string]string, fields map[string]string) {
	for field, value := range fields {
		if strings.TrimSpace(value) != "" {
			kept[field] = value
		}
	}
}

// oneLine is the first line of a preview body, for a log that stays readable.
func oneLine(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	return line
}
