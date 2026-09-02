package provider

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// maxProgramLineBytes caps one line of a program's output.
const maxProgramLineBytes = 4 << 20

// programResult is what one run of a vendor program produced.
type programResult struct {
	// text is what the model wrote.
	text string
	// usage is the token count the program reported.
	usage contract.Usage
	// cost is what the program said the call cost in dollars, or zero when it
	// said nothing.
	cost float64
	// failed says the program reported an error rather than an answer.
	failed bool
	// message is what the program said when it failed, or its answer when the
	// answer is all there is to go on.
	message string
	// sawResult says the program printed the line that ends a run, which is how
	// a run that died half way through is told from one that finished.
	sawResult bool
}

// claudeLine is one JSON value the claude program prints, wide enough to hold
// both the pieces of the streamed answer and the line that ends the run.
type claudeLine struct {
	// Type is the kind of line: "stream_event" for a piece, "result" for the end.
	Type string `json:"type"`
	// Result is the whole answer, on the line that ends the run.
	Result string `json:"result"`
	// IsError says the run ended badly.
	IsError bool `json:"is_error"`
	// TotalCostUSD is what the run cost.
	TotalCostUSD float64 `json:"total_cost_usd"`
	// Usage is the token count.
	Usage struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	} `json:"usage"`
	// Event is one piece of the answer as it was written.
	Event struct {
		Type  string `json:"type"`
		Delta struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"delta"`
	} `json:"event"`
}

// codexLine is one JSON value the codex program prints.
type codexLine struct {
	// Type is the kind of line, such as "item.completed" or "turn.completed".
	Type string `json:"type"`
	// Item is one thing the run produced, which is the answer when its own type
	// is "agent_message".
	Item struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"item"`
	// Usage is the token count, on the line that ends the turn.
	Usage struct {
		InputTokens       int `json:"input_tokens"`
		CachedInputTokens int `json:"cached_input_tokens"`
		OutputTokens      int `json:"output_tokens"`
	} `json:"usage"`
	// Message is what the program says when it gives up.
	Message string `json:"message"`
	// Error is the other shape the program reports a failure in.
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

// parseClaudeOutput reads what the claude program printed: the pieces of the
// answer as they arrive, and then the line that ends the run with the token
// counts and the cost on it.
func parseClaudeOutput(reader io.Reader, onDelta func(delta string)) programResult {
	found := programResult{}
	written := bytes.Buffer{}
	forEachJSONLine(reader, func(payload []byte) {
		line := claudeLine{}
		if err := json.Unmarshal(payload, &line); err != nil {
			return
		}
		if line.Type == "stream_event" && line.Event.Type == "content_block_delta" &&
			line.Event.Delta.Type == "text_delta" {
			if kept := addText(&written, line.Event.Delta.Text); kept != "" && onDelta != nil {
				onDelta(kept)
			}
		}
		if line.Type != "result" {
			return
		}
		found.sawResult = true
		found.failed = line.IsError
		found.message = line.Result
		found.cost = line.TotalCostUSD
		found.usage = contract.Usage{
			InputTokens:       line.Usage.InputTokens + line.Usage.CacheCreationInputTokens,
			CachedInputTokens: line.Usage.CacheReadInputTokens,
			OutputTokens:      line.Usage.OutputTokens,
		}
	})
	// A run that printed its answer only on the line that ends it still has to
	// reach the caller as a delta, so that the reply and the deltas agree.
	if !found.failed && written.Len() == 0 && found.message != "" {
		if kept := addText(&written, found.message); kept != "" && onDelta != nil {
			onDelta(kept)
		}
	}
	found.text = written.String()
	return found
}

// parseCodexOutput reads what the codex program printed: one line per thing the
// run produced, with the answer in an agent message and the counts on the line
// that ends the turn.
func parseCodexOutput(reader io.Reader, onDelta func(delta string)) programResult {
	found := programResult{}
	written := bytes.Buffer{}
	forEachJSONLine(reader, func(payload []byte) {
		line := codexLine{}
		if err := json.Unmarshal(payload, &line); err != nil {
			return
		}
		switch {
		case line.Type == "item.completed" && line.Item.Type == "agent_message":
			addText(&written, line.Item.Text)
			found.sawResult = true
		case line.Type == "turn.completed":
			found.usage = contract.Usage{
				InputTokens:       line.Usage.InputTokens,
				CachedInputTokens: line.Usage.CachedInputTokens,
				OutputTokens:      line.Usage.OutputTokens,
			}
			found.sawResult = true
		case line.Type == "error" || line.Error.Message != "":
			found.failed = true
			found.message = firstNonEmpty(line.Error.Message, line.Message)
			found.sawResult = true
		}
	})
	found.text = written.String()
	// The codex program prints nothing until it has written the whole answer, so
	// there is one delta and it is the whole of it.
	if found.text != "" && onDelta != nil {
		onDelta(found.text)
	}
	if found.message == "" {
		found.message = found.text
	}
	return found
}

// forEachJSONLine hands every line of a program's output to the function, one at
// a time, so that the pieces of an answer can be passed on as they arrive. A
// line that is not JSON is left alone: both programs print notes among their
// output, and a note is not a failure.
func forEachJSONLine(reader io.Reader, take func(payload []byte)) {
	lines := bufio.NewScanner(reader)
	lines.Buffer(make([]byte, 0, 4096), maxProgramLineBytes)
	for lines.Scan() {
		payload := bytes.TrimSpace(lines.Bytes())
		if len(payload) == 0 || payload[0] != '{' {
			continue
		}
		take(payload)
	}
}

// firstNonEmpty returns the first of the strings that says something.
func firstNonEmpty(choices ...string) string {
	for _, choice := range choices {
		if strings.TrimSpace(choice) != "" {
			return choice
		}
	}
	return ""
}
