package provider

import (
	"encoding/json"
	"testing"
)

// TestTheDaemonsOwnTimingsSayWhatWasReusedWhenTheUsageDoesNot holds the
// cost line to the daemon's own count. llama-server writes a timings object
// on the last chunk of a stream with cache_n, the tokens it read from its
// cache, and prompt_n, the tokens it had to process; in a live run its
// usage field said 5.8k tokens were cached on twelve rounds in a row while
// its log showed it had processed only one to eleven thousand of a hundred
// thousand. The timings are the truth of what the card did, so they win.
func TestTheDaemonsOwnTimingsSayWhatWasReusedWhenTheUsageDoesNot(t *testing.T) {
	building := &openAIReply{}
	chunk := openAIChunk{}
	if err := json.Unmarshal([]byte(`{"choices":[],"usage":{"prompt_tokens":100000,"completion_tokens":40,"prompt_tokens_details":{"cached_tokens":5800}},"timings":{"cache_n":90000,"prompt_n":10000,"predicted_n":40}}`), &chunk); err != nil {
		t.Fatalf("cannot read the chunk: %v", err)
	}
	if _, err := building.take(chunk); err != nil {
		t.Fatalf("the chunk was refused: %v", err)
	}
	if building.usage.CachedInputTokens != 90000 || building.usage.InputTokens != 100000 || building.usage.OutputTokens != 40 {
		t.Errorf("the usage reads %+v, want the daemon's own 90000 reused of 100000", building.usage)
	}

	without := openAIChunk{}
	if err := json.Unmarshal([]byte(`{"choices":[],"usage":{"prompt_tokens":6100,"completion_tokens":400,"prompt_tokens_details":{"cached_tokens":5200}}}`), &without); err != nil {
		t.Fatalf("cannot read the chunk: %v", err)
	}
	if _, err := building.take(without); err != nil {
		t.Fatalf("the chunk was refused: %v", err)
	}
	if building.usage.CachedInputTokens != 5200 {
		t.Errorf("with no timings the usage reads %+v, want the cached count the usage field carries", building.usage)
	}
}
