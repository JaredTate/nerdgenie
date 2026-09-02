package tui

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

// maxPictureBytes is the largest screenshot the screen will draw inline. A
// bigger file is shown as its path instead, because pushing megabytes through a
// terminal is slower than opening the file.
const maxPictureBytes = 4 << 20

// kittyChunkBytes is how much of a picture the kitty graphics protocol takes in
// one escape sequence.
const kittyChunkBytes = 4096

// pictureProtocol is the way, if any, this terminal draws a picture inline.
type pictureProtocol int

const (
	// picturesNone means the terminal cannot draw a picture, and a screenshot is
	// shown as its path.
	picturesNone pictureProtocol = iota
	// picturesKitty is the kitty graphics protocol.
	picturesKitty
	// picturesITerm is the iTerm2 inline image protocol.
	picturesITerm
)

// detectPictures reads the environment for the terminals that are known to draw
// pictures inline. Nothing is ever assumed: a terminal that does not say so is
// treated as one that cannot.
func detectPictures(environment func(string) string) pictureProtocol {
	if environment("TERM") == "xterm-kitty" || environment("KITTY_WINDOW_ID") != "" {
		return picturesKitty
	}
	switch {
	case environment("TERM_PROGRAM") == "iTerm.app",
		environment("TERM_PROGRAM") == "WezTerm",
		environment("LC_TERMINAL") == "iTerm2":
		return picturesITerm
	}
	return picturesNone
}

// pictureLine draws one screenshot inside the frame, and says false when it
// cannot, which is when the terminal has no protocol for it or the file cannot
// be read.
func (screen *Screen) pictureLine(path string, columns int) (string, bool) {
	if screen.pictures == picturesNone {
		return "", false
	}
	content, err := readPicture(path)
	if err != nil {
		return "", false
	}
	return inlinePicture(screen.pictures, content, columns), true
}

// readPicture reads a screenshot, refusing one past the cap rather than holding
// it all in memory.
func readPicture(path string) ([]byte, error) {
	about, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("cannot look at the screenshot %s, so open it yourself: %w", path, err)
	}
	if about.Size() > maxPictureBytes {
		return nil, fmt.Errorf("the screenshot %s is %d bytes and the limit for drawing one here is %d, so open it yourself", path, about.Size(), maxPictureBytes)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read the screenshot %s, so open it yourself: %w", path, err)
	}
	return content, nil
}

// inlinePicture turns a picture into the escape sequence one of the two
// protocols draws it with, scaled to a number of columns.
func inlinePicture(protocol pictureProtocol, content []byte, columns int) string {
	encoded := base64.StdEncoding.EncodeToString(content)
	if protocol == picturesITerm {
		return fmt.Sprintf("\x1b]1337;File=inline=1;width=%d:%s\a", columns, encoded)
	}
	return kittyPicture(encoded, columns)
}

// kittyPicture writes a picture as the kitty graphics protocol draws it, which
// is one escape sequence per four thousand characters with a marker on every one
// but the last saying that more is coming.
func kittyPicture(encoded string, columns int) string {
	built := strings.Builder{}
	first := true
	for len(encoded) > 0 {
		size := min(kittyChunkBytes, len(encoded))
		more := 0
		if size < len(encoded) {
			more = 1
		}
		if first {
			fmt.Fprintf(&built, "\x1b_Gf=100,a=T,c=%d,m=%d;%s\x1b\\", columns, more, encoded[:size])
			first = false
		} else {
			fmt.Fprintf(&built, "\x1b_Gm=%d;%s\x1b\\", more, encoded[:size])
		}
		encoded = encoded[size:]
	}
	return built.String()
}
