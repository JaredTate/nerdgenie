package orientation_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/orientation"
)

// theTable is a socket table in Linux's own shape: two sockets listening on
// ports 8091 and 19091, and one established connection, which is not.
const theTable = `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:1F9B 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 10597867 1 0000000000000000 100 0 0 10 0
   1: 0100007F:4A93 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 6169354 1 0000000000000000 100 0 0 10 0
   2: 0100007F:E4B2 0100007F:1F9B 01 00000000:00000000 00:00000000 00000000  1000        0 6169355 1 0000000000000000 100 0 0 10 0
`

func aTable(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tcp")
	if err := os.WriteFile(path, []byte(theTable), 0o644); err != nil {
		t.Fatalf("cannot write the table: %v", err)
	}
	return path
}

func aFolder(t *testing.T, names ...string) string {
	t.Helper()
	folder := t.TempDir()
	for _, name := range names {
		path := filepath.Join(folder, name)
		if strings.HasSuffix(name, "/") {
			if err := os.MkdirAll(path, 0o755); err != nil {
				t.Fatalf("cannot make %s: %v", name, err)
			}
			continue
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatalf("cannot write %s: %v", name, err)
		}
	}
	return folder
}

func TestTheBlockNamesTheFolderSortedWithFoldersMarked(t *testing.T) {
	folder := aFolder(t, "src/", "README.md", "package.json", "tests/")

	block := orientation.Block(orientation.Facts{Folder: folder, ProcFiles: []string{aTable(t)}})

	if !strings.HasPrefix(block, orientation.TheHeading+"\n") {
		t.Errorf("the block does not open with the heading:\n%s", block)
	}
	if !strings.Contains(block, "  README.md  package.json  src/  tests/") {
		t.Errorf("the folder's entries are not sorted with folders marked:\n%s", block)
	}
}

func TestTheFolderIsCappedAndSaysHowManyMore(t *testing.T) {
	names := []string{}
	for at := 0; at < orientation.MaxNames+5; at++ {
		names = append(names, fmt.Sprintf("file-%02d.txt", at))
	}
	folder := aFolder(t, names...)

	block := orientation.Block(orientation.Facts{Folder: folder, ProcFiles: []string{aTable(t)}})

	if !strings.Contains(block, "... and 5 more") {
		t.Errorf("the block does not say how many entries were left out:\n%s", block)
	}
	if strings.Count(block, ".txt") != orientation.MaxNames {
		t.Errorf("the block names %d entries, want %d", strings.Count(block, ".txt"), orientation.MaxNames)
	}
}

func TestAFolderThatCannotBeReadIsOneLineNotAFailure(t *testing.T) {
	block := orientation.Block(orientation.Facts{Folder: filepath.Join(t.TempDir(), "gone"), ProcFiles: []string{aTable(t)}})

	if !strings.Contains(block, "could not be read") {
		t.Errorf("a missing folder is not said in one line:\n%s", block)
	}
}

func TestTheListeningPortsAreReadFromTheTableSortedAndTheRestLeftOut(t *testing.T) {
	ports, problems := orientation.ListeningPorts(aTable(t), filepath.Join(t.TempDir(), "tcp6"))

	if len(ports) != 2 || ports[0] != 8091 || ports[1] != 19091 {
		t.Errorf("the ports are %v, want 8091 and 19091 and not the established connection", ports)
	}
	if len(problems) != 1 || !strings.Contains(problems[0], "could not be read") {
		t.Errorf("the missing table is not said: %v", problems)
	}
	block := orientation.Block(orientation.Facts{ProcFiles: []string{aTable(t)}})
	if !strings.Contains(block, "listening ports: 8091 19091") {
		t.Errorf("the ports line is wrong:\n%s", block)
	}
}

func TestTheNewestResultsRideWholeWithHeadAndTailKeptWhenLong(t *testing.T) {
	long := strings.Repeat("head ", 300) + "MIDDLE" + strings.Repeat(" tail", 300)
	block := orientation.Block(orientation.Facts{
		ProcFiles: []string{aTable(t)},
		Results:   []orientation.Result{{ID: "r41", Text: "finished with exit code 0\nall 12 tests passing"}, {ID: "r42", Text: long}},
	})

	if !strings.Contains(block, orientation.TheResultsHeading+"\nr41:\nfinished with exit code 0\nall 12 tests passing\nr42:\n") {
		t.Errorf("the short result does not ride whole under the heading:\n%s", block)
	}
	if strings.Contains(block, "MIDDLE") || !strings.Contains(block, "characters cut") {
		t.Errorf("the long result was not cut in the middle:\n%s", block)
	}
	after := block[strings.Index(block, "r42:\n")+5:]
	if !strings.HasPrefix(after, "head head") || !strings.HasSuffix(block, " tail") {
		t.Errorf("the long result lost its head or its tail:\n%s", block)
	}
}

func TestTheWholeBlockIsBounded(t *testing.T) {
	results := []orientation.Result{}
	for at := 0; at < 6; at++ {
		results = append(results, orientation.Result{ID: "r1", Text: strings.Repeat("x", orientation.MaxResultLetters)})
	}
	block := orientation.Block(orientation.Facts{ProcFiles: []string{aTable(t)}, Results: results})

	if len(block) > orientation.MaxBlockLetters+100 {
		t.Errorf("the block is %d characters, and the cap is %d", len(block), orientation.MaxBlockLetters)
	}
}

func TestAMissingTableAndAnEmptyFolderAreSaidPlainly(t *testing.T) {
	block := orientation.Block(orientation.Facts{Folder: t.TempDir(), ProcFiles: []string{filepath.Join(t.TempDir(), "none")}})

	if !strings.Contains(block, "is empty") || !strings.Contains(block, "listening ports: none (") {
		t.Errorf("the empty folder and the missing table are not said plainly:\n%s", block)
	}
}
