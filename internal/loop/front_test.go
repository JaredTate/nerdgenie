package loop_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// TestARuleTheJobAlreadyHoldsIsNotShownTwice: the standing order rides under
// the job summary, and the job summary already prints the job's rules, so a
// rule that is in both is shown once, in the summary, and the standing
// order's own rules still ride. On run eighteen every rule of the work order
// was in front of the model twice.
func TestARuleTheJobAlreadyHoldsIsNotShownTwice(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		answerStep("The game is built. What changed: the engine. What I checked: the tests. What is left: nothing."),
		answerStep("none"),
	})
	aJobFromAWorkOrder(t, built, []string{"The game is built."})
	shared := "Serve on port 8091; 8090 is in use."
	own := "Never use a headless browser."
	order := "# Tater Tots Tetris\n\n## Rules\n\n- " + loop.TheMapRule + "\n- " + shared + "\n-   " + own + "\n"
	if err := os.WriteFile(filepath.Join(built.workFolder, loop.StandingOrderFile), []byte(order), 0o644); err != nil {
		t.Fatal(err)
	}

	runTheJobToTheEnd(t, built.loop, built.channel)

	front := theTextOf(built.model.Requests()[0])
	for _, rule := range []string{shared, own, loop.TheMapRule} {
		if count := strings.Count(front, rule); count != 1 {
			t.Errorf("the rule %q is in front of the model %d times, want once; the front reads:\n%s", rule, count, front)
		}
	}
}
