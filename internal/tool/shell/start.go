package shell

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// shellProgram is what a command line is handed to when bash is not on this
// machine, because the model writes a command as a person would type it and a
// person types it at a shell.
const shellProgram = "/bin/sh"

// bashPlaces are the three places bash is looked for, in the order they are
// tried. The fence binds /usr and /bin read-only, so a bash found here is one
// the command can reach inside the fence as well as outside it.
var bashPlaces = []string{"/bin/bash", "/usr/bin/bash", "/usr/local/bin/bash"}

// CommandLine is the program one command is handed to and the arguments that go
// with it. Bash is used with pipefail set, so that a command such as
// "go test ./... | tail -40" reports the code the test run quit with rather than
// the code tail quit with; without it a model reads a failing run as a run that
// passed. A machine with no bash falls back to /bin/sh without pipefail,
// because dash cannot set it. It is exported so that a test can script a
// sandbox for the shell this machine really has.
func CommandLine(command string) (string, []string) {
	if bash := bashProgram(bashPlaces); bash != "" {
		return bash, []string{"-o", "pipefail", "-c", command}
	}
	return shellProgram, []string{"-c", command}
}

// bashProgram returns the first of the places that holds a program anybody may
// run, or the empty string when bash is on this machine nowhere.
func bashProgram(places []string) string {
	for _, place := range places {
		about, err := os.Stat(place)
		if err == nil && about.Mode().IsRegular() && about.Mode().Perm()&0o111 != 0 {
			return place
		}
	}
	return ""
}

// start runs one command: inside the fence when it is ordinary, and outside it
// through sudo when the user has approved administrator powers. Either way it
// waits ten seconds and then hands back an id rather than waiting longer.
func (tool *Tool) start(ctx context.Context, asked Call) (contract.ToolOutput, error) {
	if tool.settings.Sandbox == nil {
		return contract.ToolOutput{}, errors.New("this tool has no sandbox to run commands in, so wire the sandbox in before using it")
	}
	if err := tool.settings.Sandbox.Available(); err != nil {
		return contract.ToolOutput{}, fmt.Errorf("the shell tool is off because the sandbox cannot run, and nothing runs outside the fence: %w", err)
	}
	if asked.Escalate {
		decision, err := tool.askAboutEscalation(ctx, asked)
		if err != nil {
			return contract.ToolOutput{}, err
		}
		if decision.Ruling == contract.RulingAsk {
			return contract.ToolOutput{Text: previewText(decision, asked)}, nil
		}
		if decision.Ruling != contract.RulingAllow {
			return contract.ToolOutput{}, fmt.Errorf(
				"this command did not run with administrator powers, because the permission function ruled %q "+
					"and only the ruling %q runs a command outside the fence; run it without escalate, or ask the user again",
				decision.Ruling, contract.RulingAllow)
		}
	}

	work := tool.workOf(asked)
	entry, err := tool.running.add(asked.Command, tool.settings.Timeout, tool.now(), work)
	if err != nil {
		return contract.ToolOutput{}, err
	}
	return tool.waitOrYield(ctx, entry)
}

// workOf is the piece of work the entry runs: the fence for an ordinary command
// and sudo for one the user has approved.
func (tool *Tool) workOf(asked Call) func(ctx context.Context) (contract.SandboxResult, error) {
	if asked.Escalate {
		return func(ctx context.Context) (contract.SandboxResult, error) {
			return tool.runWithSudo(ctx, asked.Command)
		}
	}
	program, arguments := CommandLine(asked.Command)
	return func(ctx context.Context) (contract.SandboxResult, error) {
		return tool.settings.Sandbox.Run(ctx, contract.SandboxCommand{
			Program:          program,
			Arguments:        arguments,
			WorkingDirectory: tool.settings.WorkingDirectory,
			Timeout:          tool.settings.Timeout,
		})
	}
}

// waitOrYield waits for the command for as long as the yield allows and hands
// back either what it did or the id to ask after it by.
func (tool *Tool) waitOrYield(ctx context.Context, entry *entry) (contract.ToolOutput, error) {
	if tool.settings.Clock == nil {
		return contract.ToolOutput{}, errors.New("this tool has no clock to count the yield on, so wire the clock in before using it")
	}
	waiting, stopWaiting := context.WithCancel(ctx)
	defer stopWaiting()
	waited := make(chan struct{})
	go func() {
		_ = tool.settings.Clock.Sleep(waiting, YieldAfter)
		close(waited)
	}()

	select {
	case <-entry.done:
		return contract.ToolOutput{Text: entry.finishedText()}, nil
	case <-waited:
		return contract.ToolOutput{Text: entry.stillRunningText()}, nil
	case <-ctx.Done():
		return contract.ToolOutput{}, fmt.Errorf("the turn ended while %s was still running, and it is still running: %w", entry.id, ctx.Err())
	}
}

// stillRunningText is the line the model reads when a command has outlived the
// yield.
func (entry *entry) stillRunningText() string {
	return fmt.Sprintf("still running after %s as %s; poll, tail, or kill it by that id\n", YieldAfter, entry.id)
}

// finishedText is what the model reads when a command has finished: the code it
// quit with and what it wrote on each of its two streams.
func (entry *entry) finishedText() string {
	entry.guard.Lock()
	defer entry.guard.Unlock()

	if entry.err != nil {
		return fmt.Sprintf("the command could not be run: %v\n", entry.err)
	}
	if entry.result.TimedOut {
		return fmt.Sprintf("the command ran out of time after %s and was stopped\n%s", entry.timeout, streamsOf(entry.result))
	}
	return fmt.Sprintf("finished with exit code %d%s\n%s", entry.result.ExitCode, exitCodeWords(entry.result.ExitCode), streamsOf(entry.result))
}

// pipeClosedEarlyCode is what a command quits with when the command after it in
// a pipe closed its input first, which is what "head" does by design.
const pipeClosedEarlyCode = 141

// exitCodeWords is the note beside an exit code that a model would otherwise
// misread. Code 141 is a pipe closed early, which pipefail reports as the
// pipe's code even though every command in it did its job.
func exitCodeWords(code int) string {
	if code == pipeClosedEarlyCode {
		return " (a command in the pipe stopped early because the one after it needed no more, which is normal with head; treat this as success)"
	}
	return ""
}

// streamsOf is what a command wrote, each stream capped and labelled, with
// nothing at all written for a stream that is empty.
func streamsOf(result contract.SandboxResult) string {
	written := ""
	if len(result.StandardOutput) > 0 {
		written += cutStream(string(result.StandardOutput))
	}
	if len(result.StandardError) > 0 {
		written += "error output:\n" + cutStream(string(result.StandardError))
	}
	return written
}

// cutStream cuts one stream to its cap, saying how much is not shown, and makes
// sure it ends on a line of its own.
func cutStream(text string) string {
	if len(text) > MaxStreamBytes {
		text = text[:MaxStreamBytes] + fmt.Sprintf("\n... the rest is not shown; %d bytes past the cap of %d\n",
			len(text)-MaxStreamBytes, MaxStreamBytes)
	}
	if len(text) > 0 && text[len(text)-1] != '\n' {
		text += "\n"
	}
	return text
}

// timeoutOr returns the timeout to run a command under, with an hour when the
// settings name none, because every command has to end sometime.
func timeoutOr(asked time.Duration) time.Duration {
	if asked > 0 {
		return asked
	}
	return time.Hour
}

// now is the clock's time, or the zero time when no clock was given, which only
// a test does.
func (tool *Tool) now() time.Time {
	if tool.settings.Clock == nil {
		return time.Time{}
	}
	return tool.settings.Clock.Now()
}
