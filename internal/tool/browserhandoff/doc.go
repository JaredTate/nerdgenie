// Package browserhandoff is the browser_handoff tool: it brings the browser
// window forward, gives it to the user, and waits for what they say back.
//
// It is what the agent does instead of working round a login wall, a two-factor
// prompt, or a captcha. There is no captcha solving and no clever
// half-measure here: the window comes to the front, the user is told in the
// model's own words what is needed, and the task waits until they reply or the
// handoff timeout passes. Everything about how the user is reached belongs to
// the channel, which the harness wires in behind one function, so this tool has
// exactly one job: put the reason to the user and hand their answer back.
package browserhandoff
