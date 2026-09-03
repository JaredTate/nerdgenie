package tui

import (
	"io"
	"os"
	"syscall"
	"unsafe"
)

// The size the first frame is drawn at when the terminal will not say how big it
// is. Eighty by twenty-four is the size docs/TUI_DESIGN.md draws, and it is the
// oldest safe guess there is.
const (
	// fallbackWidth is how many columns the frame assumes it has.
	fallbackWidth = 80
	// fallbackHeight is how many rows the frame assumes it has.
	fallbackHeight = 24
)

// windowSize is the shape the kernel fills in when it is asked how big a
// terminal is. The first two numbers are the rows and the columns; the blank
// field stands for the size in pixels, which this screen has no use for and
// which is here only so that the shape matches what the kernel writes.
type windowSize struct {
	rows    uint16
	columns uint16
	_       [2]uint16
}

// terminalSize asks the terminal itself how big it is, and says false when the
// thing being written to is not a terminal or will not answer. Bubble Tea sends
// the real size a moment after the program starts, but it paints the first frame
// before that message arrives, so the screen has to ask once for itself.
func terminalSize(output io.Writer) (int, int, bool) {
	terminal, isFile := output.(*os.File)
	if !isFile {
		return 0, 0, false
	}
	size := windowSize{}
	_, _, failed := syscall.Syscall(
		syscall.SYS_IOCTL,
		terminal.Fd(),
		syscall.TIOCGWINSZ,
		uintptr(unsafe.Pointer(&size)),
	)
	if failed != 0 || size.columns == 0 || size.rows == 0 {
		return 0, 0, false
	}
	return int(size.columns), int(size.rows), true
}

// firstFrameSize is how big the first frame is drawn: the size the caller asked
// for when it asked for one, then the size the terminal reports, and eighty by
// twenty-four when neither says.
func firstFrameSize(options Options) (int, int) {
	if options.Width > 0 && options.Height > 0 {
		return options.Width, options.Height
	}
	reading := options.Size
	if reading == nil {
		reading = func() (int, int, bool) { return terminalSize(writerFor(options)) }
	}
	if width, height, known := reading(); known {
		return width, height
	}
	return fallbackWidth, fallbackHeight
}

// writerFor is where the frame is written, which is the standard output when the
// caller named nothing else.
func writerFor(options Options) io.Writer {
	if options.Output != nil {
		return options.Output
	}
	return os.Stdout
}
