package shell

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// shellProgram is what a command line is handed to, because the model writes a
// command as a person would type it and a person types it at a shell.
const shellProgram = "/bin/sh"

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
	entry, err := tool.running.add(asked.Command, tool.settings.Timeout, work)
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
	return func(ctx context.Context) (contract.SandboxResult, error) {
		return tool.settings.Sandbox.Run(ctx, contract.SandboxCommand{
			Program:          shellProgram,
			Arguments:        []string{"-c", asked.Command},
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
	return fmt.Sprintf("finished with exit code %d\n%s", entry.result.ExitCode, streamsOf(entry.result))
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
