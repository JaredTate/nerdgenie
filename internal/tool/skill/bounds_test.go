package skill_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/tool/skill"
)

func TestASkillFolderThatBreaksItsBoundsIsRefused(t *testing.T) {
	tool, _ := newTool(t)

	tooMany := map[string]any{}
	for at := range skill.MaxFiles + 1 {
		tooMany[fmt.Sprintf("file-%02d.md", at)] = "a line\n"
	}
	for _, broken := range []struct {
		what  string
		files map[string]any
	}{
		{"too many files", tooMany},
		{"a file with no name", map[string]any{" ": "a line\n"}},
		{"a name with a folder in it", map[string]any{"steps/one.md": "a line\n"}},
		{"a file over the size cap", map[string]any{"SKILL.md": strings.Repeat("x", skill.MaxFileBytes+1)}},
	} {
		_, err := run(t, tool, map[string]any{"action": "save", "name": "broken", "files": broken.files})
		if err == nil {
			t.Errorf("a skill folder with %s was written", broken.what)
		}
	}
}

func TestASkillThatCannotBeRunOrSavedSaysSo(t *testing.T) {
	tool, _ := newTool(t)

	if _, err := run(t, tool, map[string]any{"action": "run", "name": "no-such-skill"}); err == nil {
		t.Errorf("a skill that is not there was run")
	}
	_, err := run(t, tool, map[string]any{"action": "save", "name": "", "files": map[string]any{"SKILL.md": "x"}})
	if err == nil {
		t.Errorf("a skill with no name was saved")
	}
}
