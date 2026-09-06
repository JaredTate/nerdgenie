package loop_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// aProbe is a throwaway script written and run in one shell call, which is
// how a small model reasons out loud when it is stuck: the live build wrote
// twenty of them in a row, each a little different, and edited nothing.
func aProbe(number int, name string) contract.ToolCall {
	return callFor("c"+string(rune('a'+number)), contract.ToolShell,
		`{"command":"cd ~/game && cat > `+name+` <<'EOF'\nimport { Game } from './src/engine.js'; console.log(`+string(rune('0'+number))+`)\nEOF\nnode `+name+`"}`)
}

// TestTooManyProbesSinceTheLastEditGetOneLineBack holds the probe rule: after
// loop.MaxProbesBetweenEdits throwaway scripts with no write or edit between
// them, the next probe's result carries one line from the harness saying to
// write the failure down and change the code, and an edit starts the count
// again. The same-call guard never fires on probes, because each one differs
// by a character, so this is the rule that catches the shape.
func TestTooManyProbesSinceTheLastEditGetOneLineBack(t *testing.T) {
	steps := []testkit.Step{}
	for at := 0; at < loop.MaxProbesBetweenEdits+1; at++ {
		steps = append(steps, callStep("Let me check one more thing.", aProbe(at, "dbg.mjs")))
	}
	steps = append(steps,
		callStep("I will fix it.", callFor("cz", contract.ToolEdit, `{"path":"/game/src/engine.js","old":"a","new":"b"}`)),
		callStep("One more look.", aProbe(loop.MaxProbesBetweenEdits+1, "dbg2.mjs")),
		answerStep("Fixed. What changed: the engine. What I checked: the probes. What is left: nothing."),
	)
	outputs := []string{}
	for at := 0; at < loop.MaxProbesBetweenEdits+2; at++ {
		outputs = append(outputs, "finished with exit code 0\n"+string(rune('0'+at)))
	}
	built := newHarness(t, steps, scriptedTool(contract.ToolShell, outputs...), scriptedTool(contract.ToolEdit, "edited /game/src/engine.js by 1 line"))

	built.ask(t, "make the tests pass")

	requests := built.model.Requests()
	nudged := 0
	for _, request := range requests {
		if strings.Contains(wholeRequestText(request), loop.TheProbeLine) {
			nudged++
		}
	}
	if nudged == 0 {
		t.Fatalf("the model was never told it had run %d probes without an edit", loop.MaxProbesBetweenEdits)
	}
	early := wholeRequestText(requests[loop.MaxProbesBetweenEdits-1])
	if strings.Contains(early, loop.TheProbeLine) {
		t.Errorf("the line came before %d probes had been run", loop.MaxProbesBetweenEdits)
	}
	last := wholeRequestText(requests[len(requests)-1])
	if strings.Count(last, loop.TheProbeLine) > 1 {
		t.Errorf("the line was said more than once in one prompt, and the edit should have started the count again")
	}
}

// TestOrdinaryShellCallsAreNotProbes keeps the rule to its shape: running the
// tests, listing a folder, or building are not throwaway scripts, however
// many of them come in a row.
func TestOrdinaryShellCallsAreNotProbes(t *testing.T) {
	steps := []testkit.Step{}
	commands := []string{"node --test tests/", "ls -la", "npm run build", "node --test tests/ 2>&1 | tail -20", "git status", "node --test tests/hazards.test.js", "cat src/engine.js | head -40"}
	for at, command := range commands {
		steps = append(steps, callStep("Checking.", callFor("c"+string(rune('a'+at)), contract.ToolShell, `{"command":"`+command+`"}`)))
	}
	steps = append(steps, answerStep("All checked. What changed: nothing. What I checked: the suite. What is left: nothing."))
	outputs := make([]string, len(commands))
	for at := range outputs {
		outputs[at] = "finished with exit code 0\nok " + string(rune('0'+at))
	}
	built := newHarness(t, steps, scriptedTool(contract.ToolShell, outputs...))

	built.ask(t, "check everything")

	for _, request := range built.model.Requests() {
		if strings.Contains(wholeRequestText(request), loop.TheProbeLine) {
			t.Fatal("ordinary shell calls were counted as probes")
		}
	}
}

// TestAProbeNamedWithALeadingUnderscoreOrDotIsStillAProbe is what the fifth
// game build's play-test task showed: its throwaway scripts were named
// _probe3.js, _probe4.js and _probe5.js, and the rule looked at the start of
// the name and saw an underscore. A name is read past the marks a model puts
// in front of a file it means to hide.
func TestAProbeNamedWithALeadingUnderscoreOrDotIsStillAProbe(t *testing.T) {
	steps := []testkit.Step{}
	names := []string{"test/_probe3.js", "test/_probe4.js", "test/.tmp-check.js", "test/__debug5.mjs", "test/_probe6.js"}
	for at, name := range names {
		steps = append(steps, callStep("One more look.", aProbe(at, name)))
	}
	steps = append(steps, answerStep("Checked. What changed: nothing. What I checked: the probes. What is left: nothing."))
	outputs := []string{}
	for at := range names {
		outputs = append(outputs, "finished with exit code 0\n"+string(rune('0'+at)))
	}
	built := newHarness(t, steps, scriptedTool(contract.ToolShell, outputs...))

	built.ask(t, "make the tests pass")

	nudged := false
	for _, request := range built.model.Requests() {
		if strings.Contains(wholeRequestText(request), loop.TheProbeLine) {
			nudged = true
		}
	}
	if !nudged {
		t.Errorf("five probes named with a leading underscore or dot were never counted, and a mark in front of a name hides nothing")
	}
}

// TestAProbeWrittenWithTheWriteToolCountsToo is the fifth game build's
// play-test task on the new binary: it wrote diag2.js, diag3.js and diag4.js
// through the write tool, one after another, each a throwaway script, and
// neither the probe rule nor the meter saw a throwaway, because the rule read
// only shell heredocs and the meter counted every new file as progress.
func TestAProbeWrittenWithTheWriteToolCountsToo(t *testing.T) {
	steps := []testkit.Step{}
	writes := []string{}
	for at := 1; at <= loop.MaxProbesBetweenEdits+1; at++ {
		name := fmt.Sprintf("diag%d.js", at)
		steps = append(steps, callStep("One more diagnostic.", callFor(fmt.Sprintf("w%d", at), contract.ToolWrite,
			`{"path":"/game/`+name+`","content":"console.log(1)"}`)))
		writes = append(writes, "created /game/"+name+", 14 bytes")
	}
	steps = append(steps, answerStep("Checked. What changed: nothing. What I checked: the diagnostics. What is left: nothing."))
	checks := []string{}
	for range steps {
		checks = append(checks, "finished with exit code 0\nexit 0")
	}
	built := newHarness(t, steps, scriptedTool(contract.ToolWrite, writes...), scriptedTool(contract.ToolShell, checks...))

	built.ask(t, "find the hang")

	nudged := false
	for _, request := range built.model.Requests() {
		if strings.Contains(wholeRequestText(request), loop.TheProbeLine) {
			nudged = true
		}
	}
	if !nudged {
		t.Errorf("%d throwaway scripts written with the write tool were never counted as probes", loop.MaxProbesBetweenEdits)
	}
	last := wholeRequestText(built.model.Requests()[len(built.model.Requests())-1])
	if !strings.Contains(last, "rounds since progress: ") {
		t.Errorf("the meter reads every throwaway script as a new file and so as progress, and the request reads:\n%s", last)
	}
}
