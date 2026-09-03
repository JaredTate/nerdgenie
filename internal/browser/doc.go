// Package browser is the Go side of the browser: it starts and supervises the
// browser worker, paces it, logs in from the vault, and hands the window to the
// user.
//
// The browser is the centerpiece of the design, which is design section 9. The
// agent drives a real Google Chrome with its own profile folder, never the
// user's daily one, and the window is visible on the machine's own display.
// This package starts the TypeScript worker in worker/browser as a child
// process the first time a browser tool is used, keeps that one worker alive,
// stops it when nothing has used it for half an hour, and starts a new one when
// it dies. It speaks the JSON-RPC protocol in worker/browser/PROTOCOL.md over
// the worker's standard input and output, one request at a time with a deadline
// on each, because a browser has one window.
//
// Three rules from the design are kept here rather than in the worker. Each
// site gets a daily budget of actions, counted per hostname on the clock, and a
// call past it is refused with the number and the time the budget resets. A
// login never shows the model a secret: the vault hands over the credential,
// this package checks that the page is on one of the entry's own domains, makes
// a fresh two-factor code, and the worker types all three itself, so that no
// value is ever returned or logged. A wall the agent will not pass, which is a
// login form, a prompt for a second code, or a captcha, is handed to the user
// through the current channel with a numbered picture of the page, and the task
// waits for them to reply.
package browser
