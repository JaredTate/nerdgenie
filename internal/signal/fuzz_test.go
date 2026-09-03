package signal

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

func FuzzDecodeEvent(f *testing.F) {
	f.Add([]byte(`{"envelope":{"sourceNumber":"+1","timestamp":1,"dataMessage":{"message":"hi"}}}`))
	f.Add([]byte(`{"envelope":{"sourceNumber":"+1","syncMessage":null}}`))
	f.Add([]byte(`{"envelope":{"sourceUuid":"x","dataMessage":{"attachments":[{"id":"a"}]}}}`))
	f.Add([]byte(`{"exception":{"message":"boom"}}`))
	f.Add([]byte(`{"envelope":`))
	f.Add([]byte(``))
	f.Add([]byte(`[]`))

	f.Fuzz(func(t *testing.T, payload []byte) {
		event, isMessage := DecodeEvent(payload)
		if !isMessage {
			return
		}
		if event.Sender == "" {
			t.Errorf("the decoder read a message with no sender out of %q", payload)
		}
		if event.Text == "" && len(event.Attachments) == 0 {
			t.Errorf("the decoder read a message with nothing in it out of %q", payload)
		}
		for _, attachment := range event.Attachments {
			if attachment.ID == "" {
				t.Errorf("the decoder kept an attachment with no identifier out of %q", payload)
			}
		}
	})
}

func FuzzReadPairingCode(f *testing.F) {
	f.Add("ABCD2345")
	f.Add("abcd 2345")
	f.Add("ABCD-2345")
	f.Add("")
	f.Add("please let me in")
	f.Add(strings.Repeat("A", 1000))

	f.Fuzz(func(t *testing.T, typed string) {
		code, valid := ReadPairingCode(typed)
		if !valid {
			if code != "" {
				t.Errorf("the reader refused %q and handed back %q anyway", typed, code)
			}
			return
		}
		if len(code) != PairingCodeLength {
			t.Errorf("the reader made %q into %q, which is %d characters", typed, code, len(code))
		}
		for _, letter := range code {
			if !strings.ContainsRune(PairingCodeAlphabet, letter) {
				t.Errorf("the reader made %q into %q, which holds %q, not in the alphabet", typed, code, letter)
			}
		}
	})
}

func FuzzFrameReader(f *testing.F) {
	f.Add("data: {\"a\":1}\n\n")
	f.Add(": keepalive\n\n")
	f.Add("event: receive\ndata: one\ndata: two\n\n")
	f.Add("data\n\n")
	f.Add("")

	f.Fuzz(func(t *testing.T, stream string) {
		reader := &frameReader{}
		for _, line := range strings.Split(stream, "\n") {
			payload, ready := reader.take(line)
			if !ready {
				continue
			}
			if payload == "" {
				t.Errorf("an empty payload was handed on as an event out of %q", stream)
			}
			if len(payload) > MaxEventBytes {
				t.Errorf("a payload of %d bytes was handed on out of %q, and the cap is %d", len(payload), stream, MaxEventBytes)
			}
		}
	})
}

func FuzzSplitReply(f *testing.F) {
	f.Add("hello")
	f.Add("")
	f.Add(strings.Repeat("word ", 6000))
	f.Add(strings.Repeat("a", 3000))
	f.Add("one.\n\ntwo.\n\nthree.")
	f.Add(strings.Repeat("\U0001F600", 3000))
	f.Add(strings.Repeat("a", MessageLimit-len(contract.RedactedMarker)/2) + contract.RedactedMarker + " and the rest.")

	f.Fuzz(func(t *testing.T, reply string) {
		pieces := SplitReply(reply)
		// The reply is redacted before it is split, so a cut that lands inside
		// a marker sends half of one in each of two messages. Below the cap
		// nothing is thrown away, so every marker that went in comes out whole.
		if len(pieces) < MaxMessagesPerReply {
			whole := 0
			for _, piece := range pieces {
				whole += strings.Count(piece, contract.RedactedMarker)
			}
			if want := strings.Count(reply, contract.RedactedMarker); whole != want {
				t.Errorf("%d whole redaction markers came out of a reply holding %d, so a cut landed inside one: %q", whole, want, reply)
			}
		}
		if len(pieces) > MaxMessagesPerReply {
			t.Errorf("a reply became %d messages, and the cap is %d", len(pieces), MaxMessagesPerReply)
		}
		for at, piece := range pieces {
			if signalLength(piece) > MessageLimit {
				t.Errorf("message %d of %q is %d units long, and the limit is %d", at+1, reply, signalLength(piece), MessageLimit)
			}
			if strings.TrimSpace(piece) == "" {
				t.Errorf("message %d of %q holds nothing, and signal-cli refuses an empty message", at+1, reply)
			}
		}
	})
}
