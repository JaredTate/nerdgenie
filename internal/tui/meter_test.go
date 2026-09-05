package tui

import (
	"strings"
	"testing"
	"time"
)

// TestTheMeterFillsItsCellsByShare holds the arithmetic of a meter: a share
// of nothing is every cell empty, a full share is every cell filled, a share
// past full is capped, a share below nothing is nothing, and a share above
// nothing shows at least one filled cell, so a context in use is never drawn
// as empty.
func TestTheMeterFillsItsCellsByShare(t *testing.T) {
	for _, one := range []struct {
		share  int
		filled int
		saying string
	}{
		{0, 0, "nothing is every cell empty"},
		{1, 1, "one percent still shows one cell"},
		{5, 1, "five percent is one cell of ten"},
		{50, 5, "half is five cells"},
		{94, 9, "ninety-four is nine cells"},
		{100, 10, "full is every cell"},
		{140, 10, "past full is capped at every cell"},
		{-5, 0, "below nothing is nothing"},
	} {
		on, off := meterCells(one.share, contextMeterCells)
		if strings.Count(on, string(barFullGlyph)) != one.filled || len([]rune(on)) != one.filled {
			t.Errorf("a share of %d filled %q, and %s", one.share, on, one.saying)
		}
		if strings.Count(off, string(barEmptyGlyph)) != contextMeterCells-one.filled || len([]rune(off)) != contextMeterCells-one.filled {
			t.Errorf("a share of %d left %q empty, and the meter is %d cells", one.share, off, contextMeterCells)
		}
	}
	if contextMeterCells != 10 {
		t.Errorf("the context meter is %d cells, and the brief draws ten", contextMeterCells)
	}
}

// TestTheContextMeterIsGreenThenAmberThenRed holds the three colours of the
// context meter and where they change: green under fifty percent, amber under
// eighty, and red at eighty and over.
func TestTheContextMeterIsGreenThenAmberThenRed(t *testing.T) {
	for _, one := range []struct {
		share int
		drawn style
	}{
		{0, styleDone}, {49, styleDone}, {50, styleWarn}, {79, styleWarn}, {80, styleBad}, {100, styleBad},
	} {
		if drawn := meterStyle(one.share); drawn != one.drawn {
			t.Errorf("a share of %d is drawn in style %d, want %d", one.share, drawn, one.drawn)
		}
	}
}

// TestTheCacheShareIsTheCachedTokensOverTheContextTokens holds the number
// that found this week's speed bug: how much of the last call's input the
// provider reused, as a whole percentage, capped at a hundred, and minus one
// when either count is unknown, because a share of nothing is not a fact.
func TestTheCacheShareIsTheCachedTokensOverTheContextTokens(t *testing.T) {
	for _, one := range []struct {
		cached, context, share int
	}{
		{92000, 100000, 92}, {0, 100, 0}, {50, 0, -1}, {0, 0, -1}, {150, 100, 100}, {1, 3, 33}, {2, 3, 67},
	} {
		if share := cacheShare(one.cached, one.context); share != one.share {
			t.Errorf("%d cached of %d is a share of %d, want %d", one.cached, one.context, share, one.share)
		}
	}
	for _, one := range []struct {
		share int
		drawn style
	}{
		{100, styleDone}, {80, styleDone}, {79, styleWarn}, {40, styleWarn}, {39, styleBad}, {0, styleBad},
	} {
		if drawn := cacheStyle(one.share); drawn != one.drawn {
			t.Errorf("a cache share of %d is drawn in style %d, want %d", one.share, drawn, one.drawn)
		}
	}
}

// TestElapsedWordsAreShortAndNeverNegative holds the way a span of time is
// written in the header and the panel: seconds under a minute, whole minutes
// under an hour, hours and minutes after that, and never below nothing.
func TestElapsedWordsAreShortAndNeverNegative(t *testing.T) {
	for _, one := range []struct {
		since time.Duration
		words string
	}{
		{0, "0s"}, {42 * time.Second, "42s"}, {59 * time.Second, "59s"}, {time.Minute, "1m"},
		{12*time.Minute + 30*time.Second, "12m"}, {59 * time.Minute, "59m"}, {time.Hour, "1h 0m"},
		{time.Hour + 5*time.Minute, "1h 5m"}, {26 * time.Hour, "26h 0m"}, {-3 * time.Second, "0s"},
	} {
		if words := elapsedWords(one.since); words != one.words {
			t.Errorf("%v is written %q, want %q", one.since, words, one.words)
		}
	}
}
