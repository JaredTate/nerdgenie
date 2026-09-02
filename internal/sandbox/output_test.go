package sandbox

import (
	"strconv"
	"strings"
	"testing"
)

func TestOutputUnderTheCapComesBackExactlyAsItWasWritten(t *testing.T) {
	writer := &cappedWriter{limit: 20}

	written, err := writer.Write([]byte("hello"))
	if err != nil || written != 5 {
		t.Fatalf("writing five bytes reported %d and %v, want 5 and no error", written, err)
	}
	if string(writer.Bytes()) != "hello" {
		t.Errorf("the output is %q, want %q", writer.Bytes(), "hello")
	}
}

func TestOutputExactlyAtTheCapCarriesNoNote(t *testing.T) {
	writer := &cappedWriter{limit: 5}

	if _, err := writer.Write([]byte("hello")); err != nil {
		t.Fatalf("writing to the capped output failed: %v", err)
	}
	if string(writer.Bytes()) != "hello" {
		t.Errorf("the output is %q, want %q with no note", writer.Bytes(), "hello")
	}
}

func TestOutputPastTheCapIsDroppedWithANoteSayingHowMuch(t *testing.T) {
	writer := &cappedWriter{limit: 5}

	if _, err := writer.Write([]byte("hello, and then a good deal more")); err != nil {
		t.Fatalf("writing to the capped output failed: %v", err)
	}

	whole := string(writer.Bytes())
	if !strings.HasPrefix(whole, "hello") {
		t.Errorf("the output is %q, want it to start with the five bytes that fitted", whole)
	}
	if !strings.Contains(whole, strconv.Itoa(len("hello, and then a good deal more")-5)) {
		t.Errorf("the note in %q does not say how many bytes were dropped", whole)
	}
	if !strings.Contains(whole, strconv.Itoa(5)) {
		t.Errorf("the note in %q does not say what the cap was", whole)
	}
}

func TestOutputWrittenInManyPiecesIsCappedAcrossAllOfThem(t *testing.T) {
	writer := &cappedWriter{limit: 4}

	for _, piece := range []string{"ab", "cd", "ef", "gh"} {
		written, err := writer.Write([]byte(piece))
		if err != nil || written != len(piece) {
			t.Fatalf("writing %q reported %d and %v, want %d and no error", piece, written, err, len(piece))
		}
	}

	whole := string(writer.Bytes())
	if !strings.HasPrefix(whole, "abcd") {
		t.Errorf("the output is %q, want it to start with the four bytes that fitted", whole)
	}
	if !strings.Contains(whole, "4 more bytes") {
		t.Errorf("the note in %q does not say that four bytes were dropped", whole)
	}
}

func TestAnOutputCapOfNothingDropsEverythingAndSaysSo(t *testing.T) {
	writer := &cappedWriter{limit: 0}

	if _, err := writer.Write([]byte("anything at all")); err != nil {
		t.Fatalf("writing to the capped output failed: %v", err)
	}
	whole := string(writer.Bytes())
	if strings.Contains(whole, "anything") {
		t.Errorf("the output is %q, want nothing kept", whole)
	}
	if !strings.Contains(whole, "15 more bytes") {
		t.Errorf("the note in %q does not say that fifteen bytes were dropped", whole)
	}
}

func TestNothingWrittenComesBackAsNothing(t *testing.T) {
	writer := &cappedWriter{limit: 10}

	if len(writer.Bytes()) != 0 {
		t.Errorf("an output nobody wrote to holds %q, want nothing", writer.Bytes())
	}
}
