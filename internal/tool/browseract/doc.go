// Package browseract is the browser_act tool: a short batch of browser steps
// that stops as soon as one of them does not do what was expected.
//
// It exists so that a form with four boxes and a button costs one round rather
// than five. Every step carries what it expects to happen, exactly as the single
// action tools do, and the batch stops at the first step whose expectation was
// not met, because carrying on from a page that is not the page the model
// thought it was on is how an agent clicks the wrong thing three times in a row.
// The number of steps in one batch is capped for the same reason.
package browseract
