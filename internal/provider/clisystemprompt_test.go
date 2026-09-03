package provider_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

// systemPromptFileFlag is the flag the claude program takes the path of its
// system prompt on. The prompt is never an argument itself: every process on the
// machine can read another process's command line, and the kernel refuses a
// single argument over 128 kilobytes anyway.
const systemPromptFileFlag = "--system-prompt-file"

// firstLineOfTheSystemPrompt is a sentence from the request every test in this
// package sends, so that a test can look for the system prompt by its words.
const firstLineOfTheSystemPrompt = "You are the reasoning engine inside Coeus."

func TestTheClaudeProgramGetsItsSystemPromptInAFileAndNeverOnItsCommandLine(t *testing.T) {
	record := installFakeProgram(t, contract.ClaudeProgram, claudeFixture, 0)
	model, _, _ := commandLineModelFor(t, contract.ClaudeProgram)

	if _, err := model.Send(context.Background(), requestWithEverything(), nil); err != nil {
		t.Fatalf("one call through the claude program failed: %v", err)
	}

	arguments := record.arguments()
	for _, written := range arguments {
		if strings.Contains(written, firstLineOfTheSystemPrompt) {
			t.Errorf("the system prompt was put on the command line, where every process on this machine can read it: %q", written)
		}
	}
	path := argumentAfter(t, arguments, systemPromptFileFlag)
	written, about := record.scratchFile(t, filepath.Base(path))
	if !strings.Contains(written, firstLineOfTheSystemPrompt) {
		t.Errorf("the system prompt did not reach the program through its file:\n%s", written)
	}
	if !strings.Contains(written, contract.ToolCallOpenTag) {
		t.Errorf("the one text form of a tool call is missing from the system prompt file:\n%s", written)
	}
	if about.Mode().Perm() != contract.SecretFileMode {
		t.Errorf("the system prompt file is mode %v, want %v, because nobody else on this machine may read the context",
			about.Mode().Perm(), contract.SecretFileMode)
	}
	if held := strings.Fields(record.read("folder.txt")); len(held) != 1 || held[0] != filepath.Base(path) {
		t.Errorf("the folder the program ran in held %v, want nothing but the system prompt file %q", held, filepath.Base(path))
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("the system prompt file %s is still on disk after the call, and it holds the whole context", path)
	}
}

func TestASystemPromptLongerThanACommandLineAllowsStillReachesTheProgramWhole(t *testing.T) {
	record := installFakeProgram(t, contract.ClaudeProgram, claudeFixture, 0)
	model, _, _ := commandLineModelFor(t, contract.ClaudeProgram)
	// The kernel on this machine refuses a single argument over 128 kilobytes,
	// and a working context is easily larger than that, so this is the size that
	// used to come back as "argument list too long" instead of an answer.
	long := strings.Repeat("a", 200<<10)
	request := requestWithEverything()
	request.SystemBlocks = append(request.SystemBlocks, contract.SystemBlock{Name: "a very long block", Text: long})

	if _, err := model.Send(context.Background(), request, nil); err != nil {
		t.Fatalf("a system prompt of %d bytes could not be sent to the program: %v", len(long), err)
	}

	path := argumentAfter(t, record.arguments(), systemPromptFileFlag)
	written, _ := record.scratchFile(t, filepath.Base(path))
	if !strings.Contains(written, long) {
		t.Errorf("the long block did not reach the program whole; its file is %d bytes", len(written))
	}
}

func TestASystemPromptPastTheCapIsRefusedInWordsRatherThanByTheKernel(t *testing.T) {
	installFakeProgram(t, contract.ClaudeProgram, claudeFixture, 0)
	model, _, _ := commandLineModelFor(t, contract.ClaudeProgram)
	request := requestWithEverything()
	request.SystemBlocks = append(request.SystemBlocks, contract.SystemBlock{
		Name: "a block past every bound",
		Text: strings.Repeat("a", 1<<20),
	})

	_, err := model.Send(context.Background(), request, nil)

	if err == nil {
		t.Fatal("a system prompt past the cap was sent anyway, and the cap is there to stop it")
	}
	if !strings.Contains(err.Error(), "claude on a subscription") {
		t.Errorf("the refusal does not name the model whose prompt was too long: %v", err)
	}
	if !strings.Contains(err.Error(), "shorten") {
		t.Errorf("the refusal does not say what to do about it: %v", err)
	}
}
