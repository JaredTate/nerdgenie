package skill_test

import (
	"testing"

	"github.com/JaredTate/coeus/internal/tool/skill"
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
