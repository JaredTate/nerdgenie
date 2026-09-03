// The order of the checks follows OpenClaw's boot check at
// ~/Code/openclaw/src/gateway/boot.ts: everything the program needs is looked at
// before anything relies on it, and what is missing is reported rather than
// worked around. OpenClaw runs its check through the model; this one is ordinary
// code, so it costs nothing and cannot be talked out of a finding.

package command

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/JaredTate/coeus/internal/config"
	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/sandbox"
)

// sandboxProbeRoot is the folder the doctor builds its throwaway fence over. The
// probe makes an empty fence and quits without opening anything, so it never
// looks at the folders a real fence would carry; a fence has to be given one all
// the same, and this is a folder every Linux machine has that the sandbox rules
// never forbid.
const sandboxProbeRoot = "/usr"

// Doctor prints what config.Doctor found, adds the one thing only the sandbox
// can answer, and says whether Coeus will work.
//
// It returns false only for a problem, never for a warning: a warning means
// something Coeus can work without is switched off, such as a browser that is
// not installed, and a machine like that is set up correctly. A problem means
// something is broken, such as a configuration that will not load or a vault key
// other accounts can read, and "coeus doctor" then leaves with a failing exit
// code so that a script notices.
func Doctor(ctx context.Context, home contract.Home, output io.Writer) bool {
	report := config.Doctor(ctx, home)
	report.Findings = append(report.Findings, sandboxFinding(canReallyFence(home)))
	fmt.Fprint(output, report.String())
	return report.Verdict() != config.Trouble
}

// canReallyFence asks internal/sandbox whether this machine can build a fence.
// Looking for bwrap on the PATH is not enough, because Ubuntu ships with
// AppArmor refusing an unconfined bwrap the user namespace it needs, so the only
// honest answer comes from building an empty fence and seeing what happens.
func canReallyFence(home contract.Home) error {
	userHome, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("your home directory could not be found, and what the sandbox may reach is measured from it, so set the HOME variable: %w", err)
	}
	fence, err := sandbox.New(sandbox.Settings{
		Roots:     []string{sandboxProbeRoot},
		UserHome:  userHome,
		AgentHome: home.Root,
	})
	if err != nil {
		return err
	}
	return fence.Available()
}

// sandboxFinding turns what the sandbox said into one line of the report. A
// machine that cannot fence is not broken: with the sandbox off, which is the
// default, nothing is lost, and with it set to fence the shell tool switches
// itself off and everything else works, so this is a warning. The sandbox's own
// words are passed on whole, because they are what name the fix, which on Ubuntu
// is the AppArmor rule about unprivileged user namespaces. The fine line says the
// fence is ready for the setting that turns it on rather than that commands run
// inside it, because by default they do not; which way the setting is turned is
// config.Doctor's own finding.
func sandboxFinding(reason error) config.Finding {
	const what = "the sandbox fence"
	if reason != nil {
		return config.Finding{What: what, Result: config.Warning, Detail: reason.Error()}
	}
	return config.Finding{What: what, Result: config.Fine,
		Detail: `bwrap made a user namespace here, so the fence is ready whenever the sandbox setting is "fence"`}
}
