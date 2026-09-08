package browserread_test

import (
	"context"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
	"github.com/JaredTate/nerdgenie/internal/tool/browserread"
)

// TestAnIntentThatEvaluatesWithNoAskIsRefusedWithAnExample: five rounds of
// the flight simulator's sky task were the model saying "I need to pass an
// ask parameter" and not passing it. A read whose intent says it evaluates
// something on the page, and carries no ask, is refused with one sentence and
// one example of the field to write.
func TestAnIntentThatEvaluatesWithNoAskIsRefusedWithAnExample(t *testing.T) {
	want := `this call's intent says it evaluates something on the page, but it carries no ask; ` +
		`write the expression in the ask field, such as ask: "JSON.stringify(window.sim.state)"`
	for _, intent := range []string{
		"evaluate the sky's frame counter",
		"Diagnose why the clouds do not move",
		"run the render loop once",
		"execute the check on the page",
		"call the page's script",
		"check the JS for the camera",
		"read the JavaScript variables",
		"read the sim state",
		"read back the camera position",
	} {
		tool, _ := newTool(t)
		_, err := run(t, tool, map[string]any{"intent": intent})
		if err == nil || err.Error() != want {
			t.Errorf("the intent %q with no ask gave:\n%v\nwant:\n%s", intent, err, want)
		}
	}
}

// TestAnIntentThatOnlyReadsThePageNeedsNoAsk keeps a plain read a plain read:
// an intent that names none of the evaluating words, one that only holds them
// inside longer words, and one that names them beside an ask all go through.
func TestAnIntentThatOnlyReadsThePageNeedsNoAsk(t *testing.T) {
	for _, intent := range []string{
		"see what is on the page",
		"read the outline of the menu",
		"look at the statement on the page",
		"see the running total",
		"read the JSON the page shows",
	} {
		tool, _ := newTool(t)
		if _, err := run(t, tool, map[string]any{"intent": intent}); err != nil {
			t.Errorf("the plain read %q was refused: %v", intent, err)
		}
	}

	worker := testkit.NewFakeBrowserWorker()
	worker.AddPage(contract.Snapshot{URL: "http://localhost:8091/sim.html", Title: "Flight simulator", TabID: "t1"})
	worker.Answer("JSON.stringify(window.sim.state)", `"{\"phase\":\"flying\"}"`)
	if _, err := worker.Open(context.Background(), "http://localhost:8091/sim.html"); err != nil {
		t.Fatalf("cannot open the simulator page: %v", err)
	}
	tool := browserread.New(browserread.Settings{Browser: worker})
	output, err := run(t, tool, map[string]any{"intent": "evaluate the sim state", "ask": "JSON.stringify(window.sim.state)"})
	if err != nil {
		t.Fatalf("an evaluating intent with its ask was refused: %v", err)
	}
	if !strings.Contains(output.Text, "the page answered:") {
		t.Errorf("the ask was not answered, and the result reads:\n%s", output.Text)
	}
}
