// Package desktop is the Go side of the desktop worker: it drives one granted
// application on this machine's screen through worker/desktop.
//
// The desktop is the last resort. If the browser can do the job, the browser
// does it, which is what design section 10 says. This package starts the
// TypeScript worker as a child process the first time the desktop is used,
// keeps that one worker alive, and starts a new one by killing the old one's
// exact process identifier when it dies or says it cannot drive the machine.
// It speaks the JSON-RPC protocol in worker/desktop/PROTOCOL.md over the
// worker's standard input and output, one request at a time with a deadline on
// each, because a desktop has one mouse.
//
// Two rules from the design are enforced here rather than in the worker. The
// user grants an application once per session, through a preview on the
// channel, and nothing can be done in one until it is granted, the clipboard
// included. The one call that needs no grant is a picture of the screen,
// because looking at the screen is not acting in an application: with nothing
// open no control is numbered, the windows on the screen are named by title, so
// that the model knows what it is looking at and can launch the one it wants,
// and the picture is of the whole screen where the display allows one. The
// first human trial found the model refused a screenshot for that reason, and
// told to launch something first.
// Every action inside it that cannot be undone, which is typing into
// a field, a drag, and a paste, goes through contract.Permission and gets its
// own preview of exactly what is about to happen; so does reading the
// clipboard, because the clipboard belongs to the whole machine rather than to
// the granted window and often holds a password the user has just copied. A
// click and a key press do not, because they can be undone.
//
// Every action states what the model expected to happen and the worker checks
// it, which is the act-and-assert rule the browser uses. The expectation is
// carried on the method itself, so the caller cannot leave it behind, and an
// expectation that was not met comes back as an error carrying what the worker
// saw instead, so that the model is told rather than left to guess.
//
// The worker is handed the display and nothing else. In particular it is not
// handed the desktop's session bus, and it is told to leave the accessibility
// bridge alone, because a program handed the bus can bring that bridge up and
// on the development machine that started the screen reader and it spoke aloud.
package desktop
