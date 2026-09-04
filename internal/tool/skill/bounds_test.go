package skill_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/tool/skill"
)

// TestTheBoundsOnASkillFolderAreTheNumbersTheySay pins the two numbers this
// tool is bounded by, so that changing them is a decision and not a slip.
func TestTheBoundsOnASkillFolderAreTheNumbersTheySay(t *testing.T) {
	if skill.MaxFiles != 20 {
		t.Errorf("a skill folder may hold %d files, and the design says twenty", skill.MaxFiles)
	}
	if skill.MaxFileBytes != 256<<10 {
		t.Errorf("one skill file may hold %d bytes, and the design says 256 KiB", skill.MaxFileBytes)
	}
}

// TestASkillFolderPastItsBoundsIsRefusedByName holds the rules a folder must
// satisfy before it is written: not too many files, no file too large, no name
// that reaches outside the folder.
func TestASkillFolderPastItsBoundsIsRefusedByName(t *testing.T) {
	tool, _ := newTool(t)
	tooMany := map[string]any{}
	for at := 0; at <= skill.MaxFiles; at++ {
		tooMany[fmt.Sprintf("file-%d.md", at)] = "words"
	}
	for _, broken := range []struct {
		files map[string]any
		named string
	}{
		{tooMany, "fewer of them"},
		{map[string]any{"SKILL.md": strings.Repeat("a", skill.MaxFileBytes+1)}, "write less in it"},
		{map[string]any{"../outside.md": "words"}, "plain name"},
		{map[string]any{"": "words"}, "plain name"},
	} {
		_, err := run(t, tool, map[string]any{"action": "save", "name": "a-skill", "files": broken.files})
		if err == nil || !strings.Contains(err.Error(), broken.named) {
			t.Errorf("a folder past its bounds was not refused with %q: %v", broken.named, err)
		}
	}
}
