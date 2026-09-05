package tui

import (
	"strconv"
	"strings"
	"time"
)

// contextMeterCells is how many cells the context meter is drawn out of, in
// the header and in the side panel: ten, so that every cell is ten percent
// and a person can read the share off the meter without the number.
const contextMeterCells = 10

// The two shares at which the context meter changes colour: green while
// there is room to spare, amber from half, and red from eighty percent,
// because a person who cannot see the context filling up finds out when the
// model forgets something.
const (
	contextShareAmber = 50
	contextShareRed   = 80
)

// The two shares at which the cache share changes colour: green from eighty
// percent, amber from forty, and red below, because a cache that has gone
// cold is the first sign of the slowness this week's speed bug caused.
const (
	cacheShareGreen = 80
	cacheShareAmber = 40
)

// meterCells draws a share as the filled and the empty cells of a meter,
// handed back apart so each can be coloured on its own. The share is rounded
// to the nearest cell, a share above nothing fills at least one cell so that
// a context in use is never drawn as empty, and a share past full or below
// nothing is held to the meter.
func meterCells(share int, cells int) (string, string) {
	share = min(max(share, 0), 100)
	on := (share*cells + 50) / 100
	if share > 0 && on == 0 {
		on = 1
	}
	on = min(on, cells)
	return strings.Repeat(string(barFullGlyph), on), strings.Repeat(string(barEmptyGlyph), cells-on)
}

// meterSpans is a meter as the pieces of a row: the filled cells in the style
// given and the empty cells muted. A meter with no cells on one side or the
// other has no piece for that side.
func meterSpans(share int, cells int, filled style) []span {
	on, off := meterCells(share, cells)
	spans := []span{}
	if on != "" {
		spans = append(spans, span{style: filled, text: on})
	}
	if off != "" {
		spans = append(spans, span{style: styleMeterOff, text: off})
	}
	return spans
}

// meterStyle is the colour of the context meter and the share beside it:
// green, amber or red by how full the context is.
func meterStyle(share int) style {
	switch {
	case share >= contextShareRed:
		return styleBad
	case share >= contextShareAmber:
		return styleWarn
	default:
		return styleDone
	}
}

// contextShare is how much of the model's window the last call used, as a
// whole percentage, or minus one when either number is unknown, because a
// share of a window nobody named is not a fact the screen has.
func contextShare(tokens int, window int) int {
	if window <= 0 || tokens <= 0 {
		return -1
	}
	return min((tokens*100+window/2)/window, 100)
}

// cacheShare is how much of the last call's input the provider reused, as a
// whole percentage held to a hundred, or minus one when either count is
// unknown. A count of nothing cached is read as unknown, because a provider
// that never caches reports the same nothing as a cache that has gone cold,
// and the screen cannot tell the two apart.
func cacheShare(cached int, context int) int {
	if context <= 0 || cached <= 0 {
		return -1
	}
	return min((cached*100+context/2)/context, 100)
}

// cacheStyle is the colour of the cache share: green when warm, amber when
// only partly reused, and red when cold.
func cacheStyle(share int) style {
	switch {
	case share >= cacheShareGreen:
		return styleDone
	case share >= cacheShareAmber:
		return styleWarn
	default:
		return styleBad
	}
}

// elapsedWords writes a span of time the short way a header reads it: seconds
// under a minute, whole minutes under an hour, and hours and minutes after
// that. A span below nothing, which a clock set back can make, is nothing.
func elapsedWords(since time.Duration) string {
	if since < 0 {
		since = 0
	}
	switch {
	case since < time.Minute:
		return strconv.Itoa(int(since/time.Second)) + "s"
	case since < time.Hour:
		return strconv.Itoa(int(since/time.Minute)) + "m"
	default:
		hours := int(since / time.Hour)
		minutes := int((since - time.Duration(hours)*time.Hour) / time.Minute)
		return strconv.Itoa(hours) + "h " + strconv.Itoa(minutes) + "m"
	}
}
