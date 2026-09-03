// Package browserclick is the browser_click tool: it clicks one element and
// checks that what the model expected actually happened.
//
// The expectation is the point. A mechanic tightens a bolt and then tries to
// turn it by hand to make sure it is tight, and that hand check is what keeps
// the agent from clicking the wrong thing three times in a row. So every call
// carries what the model thinks the click will do, the worker checks it, and the
// answer says either that it happened or what happened instead, along with the
// page the click left behind.
package browserclick
