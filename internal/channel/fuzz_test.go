package channel

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

func FuzzTheSocketLineReader(f *testing.F) {
	f.Add([]byte(`{"type":"message","text":"book me a flight"}`))
	f.Add([]byte(`{"type":"command","text":"/status"}`))
	f.Add([]byte(`{"type":"approve","id":"3"}`))
	f.Add([]byte(`{"type":"secret","id":"a1","secret":"hunter2"}`))
	f.Add([]byte(`{"type":"attach"}`))
	f.Add([]byte(`{"type":"delta","text":"only the program sends this"}`))
	f.Add([]byte(`{"type":"wibble"}`))
	f.Add([]byte("   "))
	f.Add([]byte("{"))
	f.Add([]byte(`["not an object"]`))
	f.Add([]byte{0x00, 0xff, 0xfe})

	f.Fuzz(func(t *testing.T, line []byte) {
		envelope, err := decodeFromScreen(line)
		if err != nil {
			// Anything the reader refuses costs the screen its connection, which
			// is the other of the two things that may happen to a line.
			return
		}
		if !envelope.Type.FromScreen() {
			t.Fatalf("the reader accepted the type %q, which no screen sends", envelope.Type)
		}
		if envelope.Type.FromProgram() {
			t.Fatalf("the reader accepted the type %q, which only the program sends", envelope.Type)
		}

		// An envelope the reader accepted has to survive being written out and
		// read back, because that is exactly the trip every answer makes.
		written := &strings.Builder{}
		if err := contract.EncodeSocketEnvelope(written, envelope); err != nil {
			t.Fatalf("an accepted message could not be written back out: %v", err)
		}
		again, err := decodeFromScreen([]byte(written.String()))
		if err != nil {
			t.Fatalf("an accepted message could not be read back: %v", err)
		}
		if again.Type != envelope.Type || again.ID != envelope.ID || again.Text != envelope.Text {
			t.Fatalf("the message came back as %+v, want %+v", again, envelope)
		}
	})
}
