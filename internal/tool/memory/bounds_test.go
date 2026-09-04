package memory_test

import (
	"testing"

	"github.com/JaredTate/nerdgenie/internal/tool/memory"
)

// TestTheBoundOnOneSearchIsTheNumberItSays pins the one number this tool is
// bounded by, so that changing it is a decision and not a slip.
func TestTheBoundOnOneSearchIsTheNumberItSays(t *testing.T) {
	if memory.MaxFound != 10 {
		t.Errorf("one search brings back %d facts, and the design says ten", memory.MaxFound)
	}
}
