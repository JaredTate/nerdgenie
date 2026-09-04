package tool_test

import (
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/tool"
)

// TestEveryBoundOfTheRegistryIsTheNumberItSays writes each bound out as the
// literal it is, so that changing one changes a test and whoever changes it has
// to say why. The reason each number is what it is sits beside it.
func TestEveryBoundOfTheRegistryIsTheNumberItSays(t *testing.T) {
	numbers := []struct {
		name string
		is   int
		want int
		why  string
	}{
		{"MaxTools", tool.MaxTools, 64,
			"every description rides in the prompt on every call, and sixty-four of them is already a page the user pays for each turn"},
		{"MaxToolsAsked", tool.MaxToolsAsked, 64,
			"a registry holds sixty-four tools in all, so a sixty-fifth file in the folder could not be registered however well it described itself"},
		{"MaxLinkHops", tool.MaxLinkHops, 40,
			"forty links deep is further than any real path goes, and a folder pointing at itself must not become a hang"},
		{"MaxDescribeBytes", tool.MaxDescribeBytes, 64 << 10,
			"a program asked what it is can print forever, and sixty-four kilobytes is far more than a tool description needs"},
		{"MaxResultBytes", tool.MaxResultBytes, 8 << 20,
			"a tool that returns more than eight megabytes has had a run away with it, and the whole of it is no use to anybody"},
		{"MaxSpillFolderBytes", tool.MaxSpillFolderBytes, 64 << 20,
			"eight of the largest results the cap allows fit in the spill folder before the oldest of them is removed"},
	}
	for _, number := range numbers {
		if number.is != number.want {
			t.Errorf("%s is %d, and the number this package is built on is %d, because %s",
				number.name, number.is, number.want, number.why)
		}
	}
	if tool.DescribeTimeout != 10*time.Second {
		t.Errorf("DescribeTimeout is %s, want ten seconds: startup asks every program in the folder in turn, and a glance is all a description takes",
			tool.DescribeTimeout)
	}
}
