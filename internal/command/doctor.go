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

	"github.com/JaredTate/coeus/internal/config"
	"github.com/JaredTate/coeus/internal/contract"
)

// Doctor prints what config.Doctor found and says whether Coeus will work.
//
// It returns false only for a problem, never for a warning: a warning means
// something Coeus can work without is switched off, such as a browser that is
// not installed, and a machine like that is set up correctly. A problem means
// something is broken, such as a configuration that will not load or a vault key
// other accounts can read, and "coeus doctor" then leaves with a failing exit
// code so that a script notices.
func Doctor(ctx context.Context, home contract.Home, output io.Writer) bool {
	report := config.Doctor(ctx, home)
	fmt.Fprint(output, report.String())
	return report.Verdict() != config.Trouble
}
