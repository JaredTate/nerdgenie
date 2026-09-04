package provider

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// The two files a run's system prompt is written to, one per program. Each one
// is made in that run's own scratch folder and goes when the folder does.
const (
	// instructionsFileName is where the codex program's system prompt is
	// written, because that program has no flag for one and its configuration
	// takes a file.
	instructionsFileName = "instructions.txt"
	// systemPromptFileName is where the claude program's system prompt is
	// written. It goes in a file rather than on the command line because every
	// process on this machine can read another process's command line, and
	// because the kernel refuses a single argument longer than 128 kilobytes,
	// which a working context passes easily.
	systemPromptFileName = "system-prompt.txt"
)

// argumentsFor builds the command line for one run. Nothing of the conversation
// is on it: the messages go on standard input and the system prompt goes in a
// file inside the run's own scratch folder, so that no process listing and no
// shell history ever holds a word of it. The think level is on it, because both
// programs take one, and each has its own word for it.
func (model *commandLineModel) argumentsFor(folder, systemText string, level contract.Think) ([]string, error) {
	if model.alias.Program == contract.CodexProgram {
		return model.codexArguments(folder, systemText, level)
	}
	return model.claudeArguments(folder, systemText, level)
}

// claudeArguments is the command line for the claude program. The output format
// is the streaming one for every call, because it ends with the same result line
// as the plain one and it is the only way to pass the answer on as it is written.
//
// The think level goes on as --effort, which "claude --help" lists as taking
// low, medium, high, xhigh, and max. The program has no way to stop thinking at
// all, so CheckThink refuses "off" before a call is ever built.
func (model *commandLineModel) claudeArguments(folder, systemText string, level contract.Think) ([]string, error) {
	path, err := model.writeSystemPrompt(folder, systemPromptFileName, systemText)
	if err != nil {
		return nil, err
	}
	arguments := []string{
		"-p",
		"--model", model.alias.ModelName,
		"--tools", "",
		"--no-session-persistence",
		"--safe-mode",
		"--output-format", "stream-json",
		"--include-partial-messages",
		"--verbose",
		"--system-prompt-file", path,
	}
	if asksForThinking(level) {
		arguments = append(arguments, "--effort", string(level))
	}
	return arguments, nil
}

// codexArguments is the command line for the codex program. Its system prompt
// goes into a file in the scratch folder as well, because the program has no
// flag for one and its configuration has a setting that reads one.
//
// The think level goes on the same way, as the configuration override
// model_reasoning_effort, which is the only way the program takes one. Asked for
// a level it did not know, the program's own refusal named the ones it does:
// none, minimal, low, medium, high, xhigh, and max. Every level Coeus offers is
// one of those, with "off" going over as "none", which is no reasoning at all.
func (model *commandLineModel) codexArguments(folder, systemText string, level contract.Think) ([]string, error) {
	path, err := model.writeSystemPrompt(folder, instructionsFileName, systemText)
	if err != nil {
		return nil, err
	}
	arguments := []string{
		"exec",
		"--json",
		"-m", model.alias.ModelName,
		"--sandbox", "read-only",
		"--skip-git-repo-check",
		"--ephemeral",
		"--ignore-user-config",
		// The program's own shell tools are off, as the claude side's are: the
		// harness's tools are the only way a model changes anything, so every
		// change goes past the permission function and into the log.
		"--disable", "shell_tool",
		"--disable", "unified_exec",
		"-c", "model_instructions_file=" + path,
	}
	if effort := codexEffort(level); effort != "" {
		arguments = append(arguments, "-c", "model_reasoning_effort="+effort)
	}
	return arguments, nil
}

// codexEffort is what the codex program calls one think level. It is empty for
// the level that says nothing at all, which leaves the program's own default
// alone, and "none" for off, which is the program's word for no reasoning.
func codexEffort(level contract.Think) string {
	switch level {
	case contract.ThinkDefault:
		return ""
	case contract.ThinkOff:
		return "none"
	default:
		return string(level)
	}
}

// writeSystemPrompt puts one run's system prompt in that run's scratch folder,
// readable by nobody but the user the agent runs as, and returns the path to
// hand the program. The folder is removed when the call ends, and this file goes
// with it.
func (model *commandLineModel) writeSystemPrompt(folder, name, systemText string) (string, error) {
	path := filepath.Join(folder, name)
	if err := os.WriteFile(path, []byte(systemText), contract.SecretFileMode); err != nil {
		return "", fmt.Errorf("the system prompt for %s could not be written to %s: %w", model.alias.Program, path, err)
	}
	return path, nil
}
