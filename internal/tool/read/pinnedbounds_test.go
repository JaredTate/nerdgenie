package read_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/tool/read"
)

// TestEveryBoundOfTheReadToolIsTheNumberItSays writes each bound out as the
// literal it is, with the reason it is that number beside it, so that changing
// one changes a test and whoever changes it has to say why.
func TestEveryBoundOfTheReadToolIsTheNumberItSays(t *testing.T) {
	numbers := []struct {
		name string
		is   int
		want int
		why  string
	}{
		{"MaxLines", read.MaxLines, 400,
			"four hundred numbered lines is about sixteen kilobytes of ordinary code, the byte cap's worth, so the two caps meet on a real file and a longer read is paged with an offset"},
		{"MaxBytes", read.MaxBytes, 16 << 10,
			"sixteen kilobytes is about four thousand tokens, eight seconds of the local daemon's prompt processing, and the rest is read on with an offset"},
		{"MaxLineRunes", read.MaxLineRunes, 2000,
			"a file written as one enormous line must not fill the window on its own, and two thousand characters is a long line by any measure"},
		{"MaxEntries", read.MaxEntries, 1000,
			"a thousand names is more folder than anybody reads, and a longer listing says how many it left out"},
		{"ReadBufferBytes", read.ReadBufferBytes, 64 << 10,
			"sixty-four kilobytes is what is held in memory while a file of any size is walked through, and it is more than the sample the text check looks at"},
		{"MaxLineBytes", read.MaxLineBytes, 64 << 10,
			"a line longer than sixty-four kilobytes is walked past rather than held, and what is kept is far more than the characters that are shown"},
	}
	for _, number := range numbers {
		if number.is != number.want {
			t.Errorf("%s is %d, and the number this package is built on is %d, because %s",
				number.name, number.is, number.want, number.why)
		}
	}
}

// TestTheTextCheckLooksAtTheFirstEightThousandBytes brackets the sample the
// binary check reads, which is not a name this package exports: a zero byte
// inside the first eight thousand is found, and one well past them is not.
func TestTheTextCheckLooksAtTheFirstEightThousandBytes(t *testing.T) {
	tool, root := newTool(t, nil, nil)
	inside := filepath.Join(root, "inside.bin")
	held := []byte(strings.Repeat("a", 7500) + "\x00" + strings.Repeat("a", 500))
	if err := os.WriteFile(inside, held, contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the fixture file: %v", err)
	}
	if _, err := run(t, tool, map[string]any{"path": inside}); err == nil {
		t.Errorf("a zero byte 7,500 bytes in was not found, so the sample is shorter than the 8,000 bytes it should be")
	}

	beyond := filepath.Join(root, "beyond.txt")
	held = []byte(strings.Repeat("a", 9500) + "\x00" + strings.Repeat("a", 500))
	if err := os.WriteFile(beyond, held, contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the fixture file: %v", err)
	}
	if _, err := run(t, tool, map[string]any{"path": beyond}); err != nil {
		t.Errorf("a zero byte 9,500 bytes in refused the file, so the sample is longer than the 8,000 bytes it should be: %v", err)
	}
}
