// Package workorder reads an ask written under the six headings of
// PROMPT_TEMPLATE_GUIDE.md, Goal, Where, Done when, Rules, Tasks and Details,
// into the parts the record already has a place for, so that the harness can
// write a job's record itself instead of spending the model's rounds on it.
//
// An ask is a work order when it carries a Goal heading and a Done when
// heading; anything else is a plain ask and is left to the model as before. A
// done line may end with a check in brackets that the harness can run itself,
// such as "[tests pass: npm test]"; a task line may name the Details sections
// it needs, such as "(Details: Dragon, Tests required)". Nothing here rewrites
// the ask: the text is read, and the record keeps the person's words whole.
package workorder
