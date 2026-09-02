// The idea of splitting a reply at paragraph breaks and packing the parts into
// as few messages as possible was borrowed from OpenClaw's paragraph chunker at
// ~/Code/openclaw/src/auto-reply/chunk.ts, and the rule that a break is chosen
// by preference, a line break first and a word boundary second, from Hermes'
// truncate_message at ~/Code/hermes-agent/gateway/platforms/base.py. Neither
// caps the number of messages; Coeus does, because every loop has a limit.

package signal

import (
	"regexp"
	"strings"
)

const (
	// MessageLimit is how long one Signal message may be, counted the way the
	// Signal protocol counts: a character outside the basic range counts twice.
	// Past this length a Signal client turns the body into a file, which is not
	// what a reader wants, so the channel splits instead.
	MessageLimit = 2000
	// MaxMessagesPerReply is how many messages one reply may become. Ten
	// messages is already more than anybody wants on a phone, and the cap is
	// what stops a runaway reply from filling somebody's Signal.
	MaxMessagesPerReply = 10
	// TruncationNote is what the last message says when the reply was longer
	// than the cap allows.
	TruncationNote = "(cut off here: the reply was too long to send over Signal, so ask for the rest in the terminal)"
	// paragraphSeparator is the blank line between two paragraphs.
	paragraphSeparator = "\n\n"
)

// paragraphBreak matches a blank line, which may carry spaces or tabs, and is
// how one paragraph is told from the next.
var paragraphBreak = regexp.MustCompile(`\n[ \t]*\n+`)

// SplitReply cuts one reply into the fewest Signal messages that carry it,
// breaking at paragraph breaks wherever it can and never in the middle of a
// sentence while a sentence break is available. A reply that fits comes back
// unchanged as one message, a reply of nothing but spaces comes back as no
// messages at all, and a reply past the cap comes back as the cap with a note on
// the last message saying it was cut.
func SplitReply(text string) []string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}
	if signalLength(trimmed) <= MessageLimit {
		return []string{trimmed}
	}

	messages := []string{}
	building := ""
	for _, paragraph := range paragraphBreak.Split(trimmed, -1) {
		if len(messages) > MaxMessagesPerReply {
			// One message more than the cap is enough to know the rest was cut,
			// and stopping here keeps a huge reply from being laid out in full
			// only to be thrown away.
			break
		}
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}
		if signalLength(paragraph) > MessageLimit {
			if building != "" {
				messages = append(messages, building)
				building = ""
			}
			messages = append(messages, breakLongParagraph(paragraph)...)
			continue
		}
		joined := paragraph
		if building != "" {
			joined = building + paragraphSeparator + paragraph
		}
		if signalLength(joined) <= MessageLimit {
			building = joined
			continue
		}
		messages = append(messages, building)
		building = paragraph
	}
	if building != "" {
		messages = append(messages, building)
	}
	return capMessages(messages)
}

// breakLongParagraph cuts one paragraph that is longer than a message into
// pieces that each fit, preferring a sentence end and then a word boundary. It
// makes at most one piece more than the cap allows, so that a paragraph with no
// breaks in it cannot loop forever; the extra piece is what tells the cap that
// something was left over.
func breakLongParagraph(paragraph string) []string {
	pieces := []string{}
	rest := paragraph
	for range MaxMessagesPerReply + 1 {
		if signalLength(rest) <= MessageLimit {
			pieces = append(pieces, rest)
			return pieces
		}
		cut := cutPoint(rest)
		pieces = append(pieces, strings.TrimSpace(rest[:cut]))
		rest = strings.TrimLeft(rest[cut:], " \t\n")
		if rest == "" {
			return pieces
		}
	}
	return pieces
}

// cutPoint returns the byte position to cut one over-long stretch of text at: the
// end of the last whole sentence that fits, else the end of the last whole word,
// else as much as fits, always on a character boundary. A break in the first half
// of a message is ignored, because half-empty messages read worse than a break in
// an odd place.
func cutPoint(text string) int {
	end := prefixWithinUnits(text, MessageLimit)
	window := text[:end]
	if sentence := lastSentenceEnd(window); sentence*2 >= end {
		return sentence
	}
	if word := strings.LastIndexAny(window, " \t\n"); word*2 >= end {
		return word
	}
	return end
}

// lastSentenceEnd returns the byte position just after the last sentence that
// ends inside the window, or zero when no sentence ends there.
func lastSentenceEnd(window string) int {
	best := 0
	for at := range len(window) {
		if !strings.ContainsRune(".!?", rune(window[at])) {
			continue
		}
		after := at + 1
		if after == len(window) || strings.ContainsRune(" \t\n", rune(window[after])) {
			best = after
		}
	}
	return best
}

// capMessages keeps at most the cap and, when it had to drop anything, says so
// on the last message it kept, making room for the note if the message is full.
func capMessages(messages []string) []string {
	if len(messages) <= MaxMessagesPerReply {
		return messages
	}
	kept := messages[:MaxMessagesPerReply]
	room := MessageLimit - signalLength(paragraphSeparator+TruncationNote)
	last := kept[MaxMessagesPerReply-1]
	if signalLength(last) > room {
		last = strings.TrimSpace(last[:prefixWithinUnits(last, room)])
	}
	kept[MaxMessagesPerReply-1] = last + paragraphSeparator + TruncationNote
	return kept
}

// prefixWithinUnits returns the byte length of the longest prefix of the text
// that fits in the number of units, always on a character boundary.
func prefixWithinUnits(text string, units int) int {
	used := 0
	for index, letter := range text {
		width := 1
		if letter > 0xFFFF {
			width = 2
		}
		if used+width > units {
			return index
		}
		used += width
	}
	return len(text)
}

// signalLength counts the text the way the Signal protocol counts it, in
// sixteen-bit units, so that a message full of emoji is measured as Signal
// measures it rather than as Go measures it.
func signalLength(text string) int {
	length := 0
	for _, letter := range text {
		length++
		if letter > 0xFFFF {
			length++
		}
	}
	return length
}
