package shell_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
	"github.com/JaredTate/nerdgenie/internal/tool/shell"
)

// fakeSudo puts a program named sudo on the PATH that writes down its arguments
// and the one environment variable that matters, and returns the file it wrote
// them to. Nothing on this machine is ever run with real administrator powers by
// a test.
func fakeSudo(t *testing.T) string {
	t.Helper()
	folder := t.TempDir()
	written := filepath.Join(folder, "what-sudo-was-given.txt")
	script := "#!/bin/sh\n" +
		"{ echo \"arguments: $*\"; echo \"askpass: $SUDO_ASKPASS\"; } > " + written + "\n" +
		"echo 'the command ran with administrator powers'\n"
	if err := os.WriteFile(filepath.Join(folder, "sudo"), []byte(script), 0o700); err != nil {
		t.Fatalf("cannot write the fake sudo: %v", err)
	}
	t.Setenv("PATH", folder+":"+os.Getenv("PATH"))
	return written
}

// escalatingTool builds the shell tool with a nerdgenie program that is really on
// disk, so that the askpass helper it writes can point at something.
func escalatingTool(t *testing.T, permission contract.Permission) *shell.Tool {
	t.Helper()
	home := testkit.NewTempHome(t)
	program := filepath.Join(t.TempDir(), "nerdgenie")
	if err := os.WriteFile(program, []byte("#!/bin/sh\necho a-password\n"), 0o700); err != nil {
		t.Fatalf("cannot write the nerdgenie program the askpass helper points at: %v", err)
	}
	return shell.New(shell.Settings{
		Sandbox:          testkit.NewFakeSandbox(),
		Permission:       permission,
		Clock:            testkit.NewFakeClock(theMoment),
		Home:             home,
		NerdGenieProgram:     program,
		WorkingDirectory: t.TempDir(),
		Timeout:          time.Minute,
	})
}

func TestAnEscalationWithNoReasonIsRefused(t *testing.T) {
	tool := escalatingTool(t, testkit.NewFakePermission(contract.RulingAllow))

	_, err := run(t, tool, map[string]any{"command": "apt install nginx", "escalate": true})
	if err == nil {
		t.Fatalf("a command asked for administrator powers with no reason written")
	}
	if !strings.Contains(err.Error(), "reason") {
		t.Errorf("the refusal reads %q and does not ask for a reason", err)
	}
}

func TestAnEscalationThatMustBeAskedAboutPreviewsAndDoesNotRun(t *testing.T) {
	written := fakeSudo(t)
	permission := testkit.NewFakePermission(contract.RulingAllow)
	permission.Rule(contract.ToolShell, contract.PermissionDecision{
		Ruling:      contract.RulingAsk,
		Reason:      "running a command with sudo is on the ask-me-first list",
		PreviewText: "sudo apt install nginx",
	})
	tool := escalatingTool(t, permission)

	output, err := run(t, tool, map[string]any{
		"command": "apt install nginx", "escalate": true, "reason": "the web server package is missing",
	})
	if err != nil {
		t.Fatalf("an escalation that must be asked about was treated as a failure: %v", err)
	}
	if !strings.Contains(output.Text, "sudo apt install nginx") {
		t.Errorf("the preview reads %q and does not say what is about to happen", output.Text)
	}
	if !strings.Contains(output.Text, "did not run") {
		t.Errorf("the answer reads %q and does not say the command has not run", output.Text)
	}
	if _, err := os.Stat(written); !os.IsNotExist(err) {
		t.Errorf("sudo was run before the user answered the preview")
	}
	if len(permission.Requests()) != 1 {
		t.Fatalf("the permission function was asked %d times, want once", len(permission.Requests()))
	}
	if permission.Requests()[0].ToolName != contract.ToolShell {
		t.Errorf("the permission function was asked about %q, want the shell", permission.Requests()[0].ToolName)
	}
}

func TestAnApprovedEscalationRunsThroughSudoWithAnAskpassProgram(t *testing.T) {
	written := fakeSudo(t)
	tool := escalatingTool(t, testkit.NewFakePermission(contract.RulingAllow))

	output, err := run(t, tool, map[string]any{
		"command": "apt install nginx", "escalate": true, "reason": "the web server package is missing",
	})
	if err != nil {
		t.Fatalf("an approved escalation failed: %v", err)
	}
	if !strings.Contains(output.Text, "administrator powers") {
		t.Errorf("the result reads %q, want what the command wrote", output.Text)
	}

	held, err := os.ReadFile(written)
	if err != nil {
		t.Fatalf("sudo was never run: %v", err)
	}
	said := string(held)
	if !strings.Contains(said, "-A") {
		t.Errorf("sudo was given %q, and it must be given -A so it asks a program for the password", said)
	}
	if !strings.Contains(said, "apt install nginx") {
		t.Errorf("sudo was given %q, and it must be given the command the model wrote", said)
	}
	if !strings.Contains(said, "askpass: /") {
		t.Errorf("sudo was given %q, and SUDO_ASKPASS must name a program", said)
	}
}

func TestTheAskpassHelperRunsTheNerdGenieProgramAndNothingElse(t *testing.T) {
	fakeSudo(t)
	tool := escalatingTool(t, testkit.NewFakePermission(contract.RulingAllow))

	path, err := tool.AskpassHelper()
	if err != nil {
		t.Fatalf("cannot write the askpass helper: %v", err)
	}
	held, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read the askpass helper: %v", err)
	}
	if !strings.Contains(string(held), shell.AskpassSubcommand) {
		t.Errorf("the askpass helper reads %q and does not run the askpass subcommand", string(held))
	}
	about, err := os.Stat(path)
	if err != nil {
		t.Fatalf("cannot look at the askpass helper: %v", err)
	}
	if about.Mode().Perm() != 0o700 {
		t.Errorf("the askpass helper's mode is %o, and only the agent's own account may read it", about.Mode().Perm())
	}
}

func TestAnEscalationTheUserRefusedDoesNotRun(t *testing.T) {
	written := fakeSudo(t)
	permission := testkit.NewFakePermission(contract.RulingDeny)
	tool := escalatingTool(t, permission)

	_, err := run(t, tool, map[string]any{
		"command": "rm -rf /", "escalate": true, "reason": "the user asked for it",
	})
	if err == nil {
		t.Fatalf("a command the permission function refused was run")
	}
	if _, err := os.Stat(written); !os.IsNotExist(err) {
		t.Errorf("sudo was run although the permission function refused the command")
	}
}

func TestAnUnattendedEscalationStopsRatherThanWaiting(t *testing.T) {
	fakeSudo(t)
	permission := testkit.NewFakePermission(contract.RulingStop)
	tool := escalatingTool(t, permission)

	_, err := run(t, tool, map[string]any{
		"command": "apt install nginx", "escalate": true, "reason": "the package is missing",
	})
	if err == nil {
		t.Fatalf("a command that needed an answer nobody was there to give was run")
	}
	if !strings.Contains(err.Error(), "nobody") {
		t.Errorf("the refusal reads %q and does not say why nothing happened", err)
	}
}

// TestOnlyAnExplicitAllowRunsACommandWithAdministratorPowers is finding 37 of
// the wave 6 gate review. The tool refused a ruling of deny and previewed a
// ruling of ask, and ran the command for anything else, so the empty string a
// permission function returns when it has no answer, and any word a later
// rulebook might add, both ran sudo. Only the word allow may run a command
// outside the fence.
func TestOnlyAnExplicitAllowRunsACommandWithAdministratorPowers(t *testing.T) {
	for _, ruling := range []contract.PermissionRuling{"", "allow-once", "unknown"} {
		t.Run("the ruling "+string(ruling), func(t *testing.T) {
			written := fakeSudo(t)
			permission := testkit.NewFakePermission(contract.RulingAllow)
			permission.Rule(contract.ToolShell, contract.PermissionDecision{Ruling: ruling})
			tool := escalatingTool(t, permission)

			_, err := run(t, tool, map[string]any{
				"command": "apt install nginx", "escalate": true, "reason": "the web server package is missing",
			})
			if err == nil {
				t.Fatalf("a command ran with administrator powers on the ruling %q", ruling)
			}
			if !strings.Contains(err.Error(), fmt.Sprintf("%q", ruling)) {
				t.Errorf("the refusal reads %q and does not say which ruling came back", err)
			}
			if _, err := os.Stat(written); !os.IsNotExist(err) {
				t.Errorf("sudo was run on the ruling %q, which is not the word allow", ruling)
			}
		})
	}
}

func TestAnEscalationWithNoPermissionFunctionIsRefused(t *testing.T) {
	fakeSudo(t)
	tool := escalatingTool(t, nil)

	_, err := run(t, tool, map[string]any{"command": "apt install nginx", "escalate": true, "reason": "missing"})
	if err == nil {
		t.Fatalf("a command ran with administrator powers and nothing to rule on it")
	}
	if !strings.Contains(err.Error(), "permission") {
		t.Errorf("the refusal reads %q and does not say what is missing", err)
	}
}
