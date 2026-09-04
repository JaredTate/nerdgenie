package browser

import (
	"bufio"
	"bytes"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// The line reader takes whatever the worker wrote, however broken, and either
// reads it or says why. It never panics, because a worker that writes nonsense
// must not be able to take the agent down with it.
func FuzzTheProtocolLineReader(f *testing.F) {
	f.Add([]byte("{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"url\":\"https://a.test/\"}}\n"))
	f.Add([]byte("{\"jsonrpc\":\"2.0\",\"id\":null,\"error\":{\"code\":-32700,\"message\":\"not JSON\"}}\n"))
	f.Add([]byte("{\"jsonrpc\":\"2.0\",\"id\":1,\"error\":{\"code\":-32000,\"message\":\"gone\",\"data\":{\"url\":\"https://a.test/\"}}}\n"))
	f.Add([]byte("not a line at all\n"))
	f.Add([]byte("{\"id\":1,\"result\":"))
	f.Add([]byte("\n\n\n"))
	f.Add([]byte{0x00, 0xff, 0xfe, '\n'})

	f.Fuzz(func(t *testing.T, written []byte) {
		reader := bufio.NewReaderSize(bytes.NewReader(written), 16)
		for read := 0; read < 8; read++ {
			line, err := readLine(reader)
			if len(line) > 0 {
				readOneAnswerEveryWay(t, line)
			}
			if err != nil {
				return
			}
		}
	})
}

// readOneAnswerEveryWay reads one line into each of the shapes the protocol can
// answer with, so that no shape is left unfuzzed.
func readOneAnswerEveryWay(t *testing.T, line []byte) {
	t.Helper()
	var page contract.Snapshot
	if err := readAnswer(line, 1, &page); err != nil {
		if refusal, isRefusal := err.(*RefusedError); isRefusal {
			_ = refusal.Page()
			_ = refusal.needsRestart()
			_ = refusal.Error()
		}
	}
	var diff contract.Diff
	_ = readAnswer(line, 1, &diff)
	var tabs tabsAnswer
	_ = readAnswer(line, 1, &tabs)
	var diffs diffsAnswer
	_ = readAnswer(line, 1, &diffs)
	_ = diffsSoFar(&RefusedError{Code: codeNoSuchReference, Data: line})
}
