// Package computer is the computer tool: it drives the screen, the mouse, and
// the keyboard of the machine the agent runs on.
//
// It is the last resort, and its description says so: if the browser can do the
// job, the browser does it, because a web page has references and a desktop has
// only pixels. What the model sees of the screen is not a picture but a list of
// numbered controls, each with what kind of thing it is and what it is called,
// so that a step on the desktop costs about what a step in the browser costs.
// Every call says what it is for and what it expects to happen, the same way the
// browser tools do, because the desktop is where a wrong click is hardest to
// undo.
package computer
