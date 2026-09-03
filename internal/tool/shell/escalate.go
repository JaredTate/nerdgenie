// Reading the administrator password from a helper program rather than from a
// command line, an environment variable, or anything the model can see is
// Hermes' sudo path, at ~/Code/hermes-agent/tools/terminal_tool.py. The Go here
// is written fresh, and the helper is the coeus binary's own askpass subcommand.

package shell

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/JaredTate/coeus/internal/contract"
)

// AskpassSubcommand is the word the coeus binary answers with the administrator
// password, and nothing else, on its ordinary output.
const AskpassSubcommand = "askpass"

// askpassHelperName is the little program written under the run folder that runs
// the coeus binary's askpass subcommand. Sudo takes one program in SUDO_ASKPASS
// and no arguments with it, so the two words have to be wrapped in one file.
const askpassHelperName = "askpass"

// sudoProgram is what an approved command with administrator powers runs
// through. The -A tells it to read the password from the askpass program.
const sudoProgram = "sudo"

// askAboutEscalation puts the command in front of the permission function and
// returns what it ruled, refusing outright when the answer is no, or when there
// is nobody there to answer.
func (tool *Tool) askAboutEscalation(ctx context.Context, asked Call) (contract.PermissionDecision, error) {
	if tool.settings.Permission == nil {
		return contract.PermissionDecision{}, errors.New("this tool has no permission function to rule on administrator powers, so wire it in before using escalate")
	}
	written, err := json.Marshal(asked)
	if err != nil {
		return contract.PermissionDecision{}, fmt.Errorf("cannot write this call out to put it in front of the permission function: %w", err)
	}

	decision, err := tool.settings.Permission.Decide(ctx, contract.PermissionRequest{
		ToolName: contract.ToolShell,
		Input:    written,
	})
	if err != nil {
		return contract.PermissionDecision{}, fmt.Errorf("cannot find out whether this command may run with administrator powers: %w", err)
	}
	switch decision.Ruling {
	case contract.RulingDeny:
		return decision, fmt.Errorf("this command may not run with administrator powers: %s", decision.Reason)
	case contract.RulingStop:
		return decision, fmt.Errorf("this command needs somebody to approve administrator powers and nobody is there to answer: %s", decision.Reason)
	}
	return decision, nil
}

// previewText is what the user is shown, and what the model reads back, when the
// permission function ruled that a command must be asked about before it runs.
func previewText(decision contract.PermissionDecision, asked Call) string {
	preview := decision.PreviewText
	if strings.TrimSpace(preview) == "" {
		preview = sudoProgram + " " + oneLine(asked.Command)
	}
	return fmt.Sprintf(
		"this command did not run, because running it with administrator powers needs the user's approval first.\n"+
			"what it would run: %s\nwhy it needs them: %s\n", preview, oneLine(asked.Reason))
}

// runWithSudo runs an approved command outside the fence, with sudo reading the
// password from the askpass helper rather than from anything the model can see.
func (tool *Tool) runWithSudo(ctx context.Context, command string) (contract.SandboxResult, error) {
	helper, err := tool.AskpassHelper()
	if err != nil {
		return contract.SandboxResult{}, err
	}

	program, arguments := CommandLine(command)
	running := exec.CommandContext(ctx, sudoProgram, append([]string{"-A", "--", program}, arguments...)...)
	running.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	running.Dir = tool.settings.WorkingDirectory
	running.Env = []string{
		"SUDO_ASKPASS=" + helper,
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + tool.settings.Home.Root,
		"LANG=C.UTF-8",
	}
	running.Cancel = func() error { return killGroup(running) }
	written := &bytes.Buffer{}
	complaint := &bytes.Buffer{}
	running.Stdout = written
	running.Stderr = complaint

	err = running.Run()
	result := contract.SandboxResult{StandardOutput: written.Bytes(), StandardError: complaint.Bytes()}
	if quit, isQuit := err.(*exec.ExitError); isQuit {
		result.ExitCode = quit.ExitCode()
		return result, nil
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		result.TimedOut = true
		return result, nil
	}
	return result, err
}

// AskpassHelper writes the little program sudo reads the password from, under
// the run folder and readable by nobody but the agent's own account, and returns
// its path. It runs the coeus binary's askpass subcommand and nothing else.
func (tool *Tool) AskpassHelper() (string, error) {
	program := tool.settings.CoeusProgram
	if program == "" {
		found, err := os.Executable()
		if err != nil {
			return "", fmt.Errorf("cannot find the coeus program on disk to read the administrator password from: %w", err)
		}
		program = found
	}

	folder := tool.settings.Home.RunFolder()
	if err := os.MkdirAll(folder, contract.HomeFolderMode); err != nil {
		return "", fmt.Errorf("cannot make the folder %s to write the askpass helper in: %w", folder, err)
	}
	path := filepath.Join(folder, askpassHelperName)
	script := "#!/bin/sh\n# Written by the shell tool. It prints the administrator password from the vault.\nexec " +
		quoted(program) + " " + AskpassSubcommand + " \"$@\"\n"
	if err := os.WriteFile(path, []byte(script), contract.HomeFolderMode); err != nil {
		return "", fmt.Errorf("cannot write the askpass helper %s: %w", path, err)
	}
	return path, nil
}

// quoted puts a path in single quotes so that a folder with a space in it still
// runs, and refuses nothing, because the path comes from the configuration
// rather than from the model.
func quoted(path string) string {
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}

// killGroup stops a command and everything it started, because a command that
// starts children and is then stopped must not leave them behind.
func killGroup(running *exec.Cmd) error {
	if running.Process == nil {
		return nil
	}
	if err := syscall.Kill(-running.Process.Pid, syscall.SIGKILL); err != nil {
		return running.Process.Kill()
	}
	return nil
}
