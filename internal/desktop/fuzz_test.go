package desktop

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

// FuzzReadingOneAnswer throws any bytes at the reader of the worker's answers.
// The worker is another program, so everything it writes is text from outside
// this one, and nothing it writes may ever bring this one down.
func FuzzReadingOneAnswer(f *testing.F) {
	f.Add([]byte(`{"jsonrpc":"2.0","id":1,"result":{"healthy":true}}`))
	f.Add([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"there is no control numbered 9"}}`))
	f.Add([]byte(`{"jsonrpc":"2.0","id":2,"result":{"marks":[{"number":1,"role":"button","name":"OK"}]}}`))
	f.Add([]byte("not JSON at all"))
	f.Add([]byte(`{"jsonrpc":"2.0","id":1}`))
	f.Add([]byte(`{"jsonrpc":"2.0","id":1,"result":[]}`))
	f.Add([]byte{0x00, 0xff, 0xfe})

	f.Fuzz(func(t *testing.T, line []byte) {
		var picture screenshotAnswer
		if err := readAnswer(line, 1, &picture); err != nil && err.Error() == "" {
			t.Fatal("an answer was refused with an empty message, and every error must say what went wrong")
		}
		var diff diffAnswer
		_ = readAnswer(line, 2, &diff)
		var health healthAnswer
		_ = readAnswer(line, 3, &health)
	})
}

// FuzzReadingOneLine throws any bytes at the line reader, which is the first
// thing that touches whatever the worker wrote.
func FuzzReadingOneLine(f *testing.F) {
	f.Add([]byte("one line\n"))
	f.Add([]byte("no newline at all"))
	f.Add([]byte("\n\n\n"))
	f.Add(bytes.Repeat([]byte("x"), 70_000))

	f.Fuzz(func(t *testing.T, written []byte) {
		reader := bufio.NewReaderSize(bytes.NewReader(written), 64)
		for round := 0; round < 8; round++ {
			line, err := readLine(reader)
			if err != nil {
				if err.Error() == "" {
					t.Fatal("a line was refused with an empty message, and every error must say what went wrong")
				}
				return
			}
			if !strings.HasSuffix(string(line), "\n") {
				t.Fatalf("the line %q came back without its ending, and the reader promises whole lines", line)
			}
		}
	})
}
