package gate

import (
	"strings"
	"testing"
)

// badlyFormatted is a Go file gofmt would rewrite, and nothing else.
const badlyFormatted = "package example\n\nfunc Count() int {\n\t\t\treturn 0\n}\n"

// wellFormatted is the same file after gofmt has been through it.
const wellFormatted = "package example\n\nfunc Count() int {\n\treturn 0\n}\n"

func TestTheFormatCheckNamesATrackedFileThatIsNotFormatted(t *testing.T) {
	root := writeFixtureModule(t, map[string]string{
		"internal/example/example.go": badlyFormatted,
	})
	makeFixtureRepository(t, root, "internal/example/example.go")
	script := copyScript(t, root, "gofmt.sh")

	printed, code := runScript(t, script)

	if code == 0 {
		t.Fatalf("a file that is not gofmt clean passed the check:\n%s", printed)
	}
	if !strings.Contains(printed, "internal/example/example.go") {
		t.Errorf("the failure does not name the file to format:\n%s", printed)
	}
}

func TestTheFormatCheckLooksOnlyAtTheFilesThisRepositoryTracks(t *testing.T) {
	root := writeFixtureModule(t, map[string]string{
		"internal/example/example.go":                    wellFormatted,
		".claude/worktrees/someone-else/internal/bad.go": badlyFormatted,
	})
	makeFixtureRepository(t, root, "internal/example/example.go")
	script := copyScript(t, root, "gofmt.sh")

	printed, code := runScript(t, script)

	if code != 0 {
		t.Fatalf("the check failed on a file in another worker's worktree, which is not this branch's to format:\n%s", printed)
	}
	if strings.Contains(printed, "someone-else") {
		t.Errorf("the check looked inside another worker's worktree:\n%s", printed)
	}
}

func TestTheFormatCheckHandlesAPathWithASpaceInIt(t *testing.T) {
	root := writeFixtureModule(t, map[string]string{
		"internal/a folder/example.go": badlyFormatted,
	})
	makeFixtureRepository(t, root, "internal/a folder/example.go")
	script := copyScript(t, root, "gofmt.sh")

	printed, code := runScript(t, script)

	if code == 0 {
		t.Fatalf("a badly formatted file whose folder has a space in it passed the check:\n%s", printed)
	}
	if !strings.Contains(printed, "a folder/example.go") {
		t.Errorf("the failure does not name the file with the space in its path:\n%s", printed)
	}
}

func TestTheFormatCheckFailsWhenGofmtCannotReadAFile(t *testing.T) {
	root := writeFixtureModule(t, map[string]string{
		"internal/example/example.go": "package ???\n",
	})
	makeFixtureRepository(t, root, "internal/example/example.go")
	script := copyScript(t, root, "gofmt.sh")

	printed, code := runScript(t, script)

	if code == 0 {
		t.Fatalf("gofmt could not read a file and the check reported success anyway:\n%s", printed)
	}
}
