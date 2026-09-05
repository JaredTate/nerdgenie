// Package tui is the terminal screen: a thin client that draws what the running
// program sends over the local socket and sends back what the person types.
//
// The screen holds no state of its own beyond what is on it. It keeps a link to
// the program at the socket path internal/contract names, speaking one JSON
// object per line in the shapes that package defines, and it draws one frame: a
// header, a rule, the transcript, a rule, the input box, and a status strip. The
// frame is painted edge to edge on the DigiByte dark blue ground, with the
// person's words on a flat card leaning right and the agent's on one leaning
// left, each with a coloured bar down its side, each tool call as a compact
// pill with a glyph for its state, a side panel down the right-hand side
// holding the model, what is happening now, the checklist of the job and its
// tasks and the record's own state, a row of key hints under the input box,
// and the NERD GENIE wordmark in block letters while there is nothing to show
// yet. The frame is drawn before the link is made, so the person sees the
// screen at once, and the status strip tells the truth about the link the
// whole time. The drawing this package implements is docs/TUI_DESIGN.md as
// the second look reworked it, and every rule in it, from the
// five-hundred-millisecond spinner delay to the masked prompt that never
// echoes what is typed, is a test in this folder.
package tui
