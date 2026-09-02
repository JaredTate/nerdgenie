package provider_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/provider"
	"github.com/JaredTate/coeus/internal/testkit"
)

// The two fixtures below are the shapes the real programs print, taken from one
// run of each on the development machine.
const claudeFixture = `{"type":"system","subtype":"init","tools":[]}
{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Reading "}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"the notes."}}}
{"type":"result","subtype":"success","is_error":false,"result":"Reading the notes.","total_cost_usd":0.00239,"usage":{"input_tokens":264,"cache_creation_input_tokens":10,"cache_read_input_tokens":100,"output_tokens":5}}
`

const codexFixture = `{"type":"thread.started","thread_id":"01a0642a"}
{"type":"turn.started"}
{"type":"item.completed","item":{"id":"item_0","type":"agent_message","text":"Reading the notes."}}
{"type":"turn.completed","usage":{"input_tokens":8565,"cached_input_tokens":4480,"cache_write_input_tokens":0,"output_tokens":2,"reasoning_output_tokens":0}}
`

// programRecord is what a stand-in program wrote down about the run it was given.
type programRecord struct {
	folder string
}

// read returns one of the files the stand-in wrote, or an empty string.
func (record programRecord) read(name string) string {
	body, err := os.ReadFile(filepath.Join(record.folder, name))
	if err != nil {
		return ""
	}
	return string(body)
}

// argumentSeparator is what the stand-in writes between the arguments it was
// given, because one of them is a whole system prompt with newlines in it.
const argumentSeparator = "\n--- next argument ---\n"

// arguments are the arguments the stand-in was given, whole.
func (record programRecord) arguments() []string {
	written := record.read("args.txt")
	return strings.Split(strings.TrimSuffix(written, argumentSeparator), argumentSeparator)
}

// installFakeProgram writes a stand-in for one of the vendor programs onto a
// folder at the front of the PATH. It writes down its arguments, its working
// folder, what that folder held, and everything it was given on standard input,
// then prints the output and exits with the code.
func installFakeProgram(t *testing.T, name, output string, exitCode int) programRecord {
	t.Helper()
	binFolder := t.TempDir()
	recordFolder := t.TempDir()
	if err := os.WriteFile(filepath.Join(recordFolder, "stdout.txt"), []byte(output), 0o600); err != nil {
		t.Fatalf("writing the stand-in's output failed: %v", err)
	}
	script := fmt.Sprintf(`#!/bin/sh
printf '%%s%s' "$@" > %q/args.txt
pwd > %q/cwd.txt
ls -A > %q/folder.txt
cat > %q/stdin.txt
cat %q/stdout.txt
exit %d
`, strings.ReplaceAll(argumentSeparator, "\n", "\\n"),
		recordFolder, recordFolder, recordFolder, recordFolder, recordFolder, exitCode)
	if err := os.WriteFile(filepath.Join(binFolder, name), []byte(script), 0o700); err != nil {
		t.Fatalf("writing the stand-in program failed: %v", err)
	}
	t.Setenv("PATH", binFolder+string(os.PathListSeparator)+os.Getenv("PATH"))
	return programRecord{folder: recordFolder}
}

// commandLineModelFor builds the command-line provider for one program.
func commandLineModelFor(t *testing.T, program string) (contract.Model, provider.Options, *noteRecorder) {
	t.Helper()
	options, recorder := testOptions(t, newTestClock())
	model, err := provider.New(contract.ModelAlias{
		Name:          program + " on a subscription",
		Provider:      contract.ProviderCommandLine,
		Program:       program,
		ModelName:     "a-model",
		ContextLength: 200000,
	}, options)
	if err != nil {
		t.Fatalf("building the command-line provider for %s failed: %v", program, err)
	}
	return model, options, recorder
}

func TestTheClaudeProgramsTextComesBackAsTheReplyWithItsUsageAndCost(t *testing.T) {
	record := installFakeProgram(t, contract.ClaudeProgram, claudeFixture, 0)
	model, _, recorder := commandLineModelFor(t, contract.ClaudeProgram)

	reply, streamed, err := sendAndCollect(context.Background(), model, requestWithEverything())

	if err != nil {
		t.Fatalf("one call through the claude program failed: %v", err)
	}
	if reply.Text != "Reading the notes." || streamed != reply.Text {
		t.Errorf("the reply is %q and the deltas joined to %q, want the program's text in both", reply.Text, streamed)
	}
	want := contract.Usage{InputTokens: 274, CachedInputTokens: 100, OutputTokens: 5}
	if reply.Usage != want {
		t.Errorf("the usage came back as %+v, want %+v", reply.Usage, want)
	}
	if len(reply.ToolCalls) != 0 {
		t.Errorf("the reply carries %d tool calls, and a program that returns text carries none until repair reads them",
			len(reply.ToolCalls))
	}
	if !strings.Contains(strings.Join(recorder.all(), "\n"), "0.00239") {
		t.Errorf("the cost the program printed was not written down: %v", recorder.all())
	}
	_ = record
}

func TestTheClaudeProgramIsGivenTheSystemPromptAndThePromptOnStandardInput(t *testing.T) {
	record := installFakeProgram(t, contract.ClaudeProgram, claudeFixture, 0)
	model, options, _ := commandLineModelFor(t, contract.ClaudeProgram)

	if _, err := model.Send(context.Background(), requestWithEverything(), nil); err != nil {
		t.Fatalf("one call through the claude program failed: %v", err)
	}

	arguments := record.arguments()
	joined := strings.Join(arguments, " ")
	for _, wanted := range []string{"-p", "--model", "a-model", "--tools", "--no-session-persistence", "--system-prompt"} {
		if !strings.Contains(joined, wanted) {
			t.Errorf("the program was not given %q:\n%s", wanted, joined)
		}
	}
	systemPrompt := argumentAfter(t, arguments, "--system-prompt")
	if !strings.Contains(systemPrompt, "You are the reasoning engine inside Coeus.") {
		t.Errorf("the system prompt was not passed to the program:\n%s", systemPrompt)
	}
	if !strings.Contains(systemPrompt, contract.ToolCallOpenTag) {
		t.Errorf("the one text form of a tool call is missing from the system prompt:\n%s", systemPrompt)
	}
	given := record.read("stdin.txt")
	if !strings.Contains(given, "Post the tweet about the launch.") {
		t.Errorf("the conversation did not reach the program on standard input:\n%s", given)
	}
	if strings.Contains(joined, "Post the tweet about the launch.") {
		t.Error("the prompt was put on the command line, and it belongs on standard input")
	}
	assertRanInAnEmptyFolderUnder(t, record, options)
}

func TestTheToolSpecsAndTheirFieldsReachTheProgram(t *testing.T) {
	record := installFakeProgram(t, contract.ClaudeProgram, claudeFixture, 0)
	model, _, _ := commandLineModelFor(t, contract.ClaudeProgram)

	if _, err := model.Send(context.Background(), requestWithEverything(), nil); err != nil {
		t.Fatalf("one call through the claude program failed: %v", err)
	}

	written := argumentAfter(t, record.arguments(), "--system-prompt")
	for _, wanted := range []string{contract.ToolRead, contract.ToolWrite, "path", "string", "The file to read."} {
		if !strings.Contains(written, wanted) {
			t.Errorf("the tool block does not mention %q:\n%s", wanted, written)
		}
	}
}

func TestTheToolResultsReachTheProgramInsideTheirMarkers(t *testing.T) {
	record := installFakeProgram(t, contract.ClaudeProgram, claudeFixture, 0)
	model, _, _ := commandLineModelFor(t, contract.ClaudeProgram)

	if _, err := model.Send(context.Background(), requestWithEverything(), nil); err != nil {
		t.Fatalf("one call through the claude program failed: %v", err)
	}

	given := record.read("stdin.txt")
	if !strings.Contains(given, "The launch is on Friday.") {
		t.Errorf("the tool result did not reach the program:\n%s", given)
	}
	if !strings.Contains(given, "call_1") {
		t.Errorf("the transcript does not say which call the result answers:\n%s", given)
	}
	if !strings.Contains(given, contract.ToolCallOpenTag) {
		t.Errorf("the call the model already made is not written in the one text form:\n%s", given)
	}
}

func TestTheCodexProgramGetsItsSystemPromptInAnInstructionsFile(t *testing.T) {
	record := installFakeProgram(t, contract.CodexProgram, codexFixture, 0)
	model, options, _ := commandLineModelFor(t, contract.CodexProgram)

	reply, streamed, err := sendAndCollect(context.Background(), model, requestWithEverything())

	if err != nil {
		t.Fatalf("one call through the codex program failed: %v", err)
	}
	if reply.Text != "Reading the notes." || streamed != reply.Text {
		t.Errorf("the reply is %q and the deltas joined to %q, want the program's text in both", reply.Text, streamed)
	}
	want := contract.Usage{InputTokens: 8565, CachedInputTokens: 4480, OutputTokens: 2}
	if reply.Usage != want {
		t.Errorf("the usage came back as %+v, want %+v", reply.Usage, want)
	}
	joined := strings.Join(record.arguments(), " ")
	for _, wanted := range []string{"exec", "--json", "--sandbox read-only", "--skip-git-repo-check", "--ephemeral", "model_instructions_file"} {
		if !strings.Contains(joined, wanted) {
			t.Errorf("the program was not given %q:\n%s", wanted, joined)
		}
	}
	if !strings.Contains(record.read("folder.txt"), "instructions") {
		t.Errorf("the instructions file was not written into the scratch folder: %q", record.read("folder.txt"))
	}
	assertFolderIsUnderTheRunFolder(t, record, options)
}

func TestAProgramThatSpeaksOfAUsageLimitBecomesTheRateLimitSentinel(t *testing.T) {
	installFakeProgram(t, contract.ClaudeProgram,
		`{"type":"result","subtype":"error","is_error":true,"result":"Claude AI usage limit reached. Your limit resets at 5pm."}`+"\n", 1)
	model, _, _ := commandLineModelFor(t, contract.ClaudeProgram)

	_, err := model.Send(context.Background(), requestWithEverything(), nil)

	limited := contract.RateLimitedError{}
	if !errors.As(err, &limited) {
		t.Fatalf("a program that reported a usage limit came back as %v, want the rate-limit sentinel", err)
	}
	if limited.RetryAfter <= 0 {
		t.Errorf("the rate-limit sentinel asks the caller to wait %s, and a program says no time, so a fixed one is used",
			limited.RetryAfter)
	}
}

func TestAProgramThatSpeaksOfAPromptTooLongBecomesTheOverflowSentinel(t *testing.T) {
	installFakeProgram(t, contract.ClaudeProgram,
		`{"type":"result","subtype":"error","is_error":true,"result":"prompt is too long: 300000 tokens > 200000 maximum"}`+"\n", 0)
	model, _, _ := commandLineModelFor(t, contract.ClaudeProgram)

	_, err := model.Send(context.Background(), requestWithEverything(), nil)

	if !errors.Is(err, contract.ErrContextOverflow) {
		t.Fatalf("a program that reported a prompt too long came back as %v, want the overflow sentinel", err)
	}
}

func TestAProgramThatSaysNothingUsefulIsAnErrorNamingIt(t *testing.T) {
	installFakeProgram(t, contract.ClaudeProgram, "this is not JSON at all\n", 2)
	model, _, _ := commandLineModelFor(t, contract.ClaudeProgram)

	_, err := model.Send(context.Background(), requestWithEverything(), nil)

	if err == nil {
		t.Fatal("a program that printed nothing useful and failed came back as a good reply")
	}
	if !strings.Contains(err.Error(), contract.ClaudeProgram) {
		t.Errorf("the error does not name the program that failed: %v", err)
	}
}

func TestAProgramThatHangsIsKilledByItsExactProcessIdOnTheDeadline(t *testing.T) {
	binFolder := t.TempDir()
	recordFolder := t.TempDir()
	script := fmt.Sprintf(`#!/bin/sh
echo $$ > %q/pid.txt
sleep 300 &
echo $! > %q/child.txt
cat > /dev/null
wait
`, recordFolder, recordFolder)
	if err := os.WriteFile(filepath.Join(binFolder, contract.ClaudeProgram), []byte(script), 0o700); err != nil {
		t.Fatalf("writing the hanging stand-in failed: %v", err)
	}
	t.Setenv("PATH", binFolder+string(os.PathListSeparator)+os.Getenv("PATH"))
	model, _, _ := commandLineModelFor(t, contract.ClaudeProgram)
	record := programRecord{folder: recordFolder}
	ctx, giveUp := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer giveUp()

	_, err := model.Send(ctx, requestWithEverything(), nil)

	if err == nil {
		t.Fatal("a program that never finished came back as a good reply")
	}
	waitForProcessToGo(t, record, "pid.txt")
	waitForProcessToGo(t, record, "child.txt")
}

// waitForProcessToGo waits until the process whose identifier the stand-in wrote
// down is gone, which is how the test proves the whole group was killed.
func waitForProcessToGo(t *testing.T, record programRecord, file string) {
	t.Helper()
	written := strings.TrimSpace(record.read(file))
	if written == "" {
		t.Fatalf("the stand-in wrote no process id into %s, so the kill cannot be checked", file)
	}
	identifier, err := strconv.Atoi(written)
	if err != nil {
		t.Fatalf("the process id in %s is %q, which is not a number: %v", file, written, err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if syscall.Kill(identifier, 0) != nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("the process %d from %s is still running after the deadline passed", identifier, file)
}

func TestACommandLineAliasNamingAnUnknownProgramIsRefused(t *testing.T) {
	options, _ := testOptions(t, newTestClock())

	_, err := provider.New(contract.ModelAlias{
		Name:          "something else",
		Provider:      contract.ProviderCommandLine,
		Program:       "a-program-nobody-has",
		ModelName:     "a-model",
		ContextLength: 1000,
	}, options)

	if err == nil {
		t.Fatal("an alias naming a program this package cannot drive was accepted")
	}
	if !strings.Contains(err.Error(), contract.ClaudeProgram) {
		t.Errorf("the error does not say which programs there are: %v", err)
	}
}

func TestACommandLineAliasWhoseProgramIsNotInstalledIsRefusedByName(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	options, _ := testOptions(t, newTestClock())

	_, err := provider.New(contract.ModelAlias{
		Name:          "opus",
		Provider:      contract.ProviderCommandLine,
		Program:       contract.ClaudeProgram,
		ModelName:     "a-model",
		ContextLength: 1000,
	}, options)

	if err == nil {
		t.Fatal("an alias whose program is not installed was accepted")
	}
	if !strings.Contains(err.Error(), contract.ClaudeProgram) {
		t.Errorf("the error does not name the program that is missing: %v", err)
	}
}

func TestTheCommandLineProviderPassesTheContractCheck(t *testing.T) {
	installFakeProgram(t, contract.ClaudeProgram, claudeFixture, 0)
	model, _, _ := commandLineModelFor(t, contract.ClaudeProgram)

	if err := testkit.CheckModel(context.Background(), model); err != nil {
		t.Fatalf("the command-line provider does not keep the model contract: %v", err)
	}
}

// argumentAfter returns the argument that follows a flag.
func argumentAfter(t *testing.T, arguments []string, flag string) string {
	t.Helper()
	for at, written := range arguments {
		if written == flag && at+1 < len(arguments) {
			return arguments[at+1]
		}
	}
	t.Fatalf("the program was never given %q: %v", flag, arguments)
	return ""
}

// assertRanInAnEmptyFolderUnder checks that the program was run somewhere with
// nothing in it, under the home's run folder, so that no project settings, no
// CLAUDE.md, and no hooks could reach the prompt.
func assertRanInAnEmptyFolderUnder(t *testing.T, record programRecord, options provider.Options) {
	t.Helper()
	assertFolderIsUnderTheRunFolder(t, record, options)
	if held := strings.TrimSpace(record.read("folder.txt")); held != "" {
		t.Errorf("the program was run in a folder holding %q, and it must be empty", held)
	}
}

// assertFolderIsUnderTheRunFolder checks where the program was run.
func assertFolderIsUnderTheRunFolder(t *testing.T, record programRecord, options provider.Options) {
	t.Helper()
	where := strings.TrimSpace(record.read("cwd.txt"))
	runFolder, err := filepath.EvalSymlinks(options.Home.RunFolder())
	if err != nil {
		t.Fatalf("the run folder %s cannot be read: %v", options.Home.RunFolder(), err)
	}
	if !strings.HasPrefix(where, runFolder) {
		t.Errorf("the program was run in %q, and it must be under the run folder %q", where, runFolder)
	}
	if where == runFolder {
		t.Errorf("the program was run in the run folder itself, and it needs a folder of its own: %q", where)
	}
}
