package loop

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// A page whose script never yields. The browser tool says so, and says to
// look for a loop whose condition never changes; the fifth game build's
// play-test task heard that and wrote nine play-test drivers instead of
// reading the three while loops in the one file it had written. The harness
// can do the looking: when a browser result reports a hung page, it lists the
// loops in the scripts this task changed, by file and line, on the same
// result, so the model reads three lines rather than guessing for two hours.

// TheLoopsLine opens the list of loops on a hung page's result.
const TheLoopsLine = "the loops in the scripts this task changed, one of which may be the one that never yields:"

// theSignsOfAHungPage are the words a browser result carries when the page's
// own script kept it busy.
var theSignsOfAHungPage = []string{"keeping it busy", "does not yield"}

// theScriptExtensions are the files a page runs.
var theScriptExtensions = map[string]bool{".js": true, ".mjs": true, ".cjs": true, ".ts": true}

// MaxLoopsListed is the most loop lines the list holds.
const MaxLoopsListed = 12

// listTheLoopsAfter runs one search for loops over the scripts this task
// changed when a browser result reports a hung page, and hands back the list
// to put on the result, or nothing when the result is ordinary, no script was
// changed, or there is no shell tool.
func (running *run) listTheLoopsAfter(ctx context.Context, call contract.ToolCall, text string) string {
	if !strings.HasPrefix(call.Name, "browser") || !saysThePageHung(text) {
		return ""
	}
	scripts := []string{}
	for _, path := range running.filesChanged {
		if theScriptExtensions[strings.ToLower(filepath.Ext(path))] {
			scripts = append(scripts, quotedForTheShell(wholePath(path)))
		}
	}
	if len(scripts) == 0 {
		return ""
	}
	shell, found := running.tools().Lookup(contract.ToolShell)
	if !found {
		return ""
	}
	command := "grep -n -E 'while *\\(|for *\\(' " + strings.Join(scripts, " ") + " | head -" + strconv.Itoa(MaxLoopsListed)
	arguments, err := json.Marshal(map[string]string{"command": command})
	if err != nil {
		return ""
	}
	output, err := running.underTheTimeLimit(ctx, shell, contract.ToolCall{ID: call.ID + "-loops", Name: contract.ToolShell, Input: arguments})
	if err != nil {
		return ""
	}
	lines := []string{}
	for _, line := range strings.Split(output.Text, "\n") {
		if strings.Contains(line, ":") && (strings.Contains(line, "while") || strings.Contains(line, "for")) && !strings.HasPrefix(line, "finished with") {
			lines = append(lines, strings.TrimSpace(line))
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return TheLoopsLine + "\n" + strings.Join(lines, "\n")
}

// saysThePageHung says whether a browser result reports a page whose own
// script kept it busy.
func saysThePageHung(text string) bool {
	for _, sign := range theSignsOfAHungPage {
		if strings.Contains(text, sign) {
			return true
		}
	}
	return false
}
