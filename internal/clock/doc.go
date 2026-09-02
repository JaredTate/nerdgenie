// Package clock is the real clock behind contract.Clock: the machine's own time,
// a sleep that stops when its context ends, and a ticker.
//
// Every other package reads the time through contract.Clock so that a test can
// drive it with the fake in internal/testkit. This package is the one place the
// standard library's time is read for real, and serve.go hands it to everything
// that needs one.
package clock
