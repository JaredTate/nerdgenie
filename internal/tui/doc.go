// Package tui is the terminal screen: a thin client that draws what the running
// program sends over the local socket and sends back what the person types.
//
// The screen holds no state of its own beyond what is on it. It keeps a link to
// the program at the socket path internal/contract names, speaking one JSON
// object per line in the shapes that package defines, and it draws one frame: a
// header, a rule, the transcript, a rule, the input box, and a status strip. The
// frame is painted edge to edge on the DigiByte blue ground, with the person's
// words in a bubble leaning right, the agent's in a bubble leaning left, each
// tool call as a small filled pill, and the COEUS AGENT wordmark in block
// letters while there is nothing to show yet. The frame is drawn before the link
// is made, so the person sees the screen at once, and the status strip tells the
// truth about the link the whole time. The drawing this package implements is
// docs/TUI_DESIGN.md, and every rule in it, from the five-hundred-millisecond
// spinner delay to the masked prompt that never echoes what is typed, is a test
// in this folder.
package tui
