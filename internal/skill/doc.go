// Package skill is the agent's recipe box: one folder per saved procedure, with
// the words that trigger it, the steps to replay, a dry run, and a changelog.
//
// A skill folder holds four files. SKILL.md carries the skill's name, its
// one-line description, the words that trigger it, and its permissions block,
// which names the browser profile it may use, the websites it may visit, its
// daily limit, and which of its steps cannot be undone. Either steps.md or an
// executable named script carries the procedure: steps.md is a numbered list
// where each step says what it is for, which tool it calls, what to give that
// tool, and what to expect back. test.md is the dry run, which replays the
// steps up to the first one that cannot be undone and then stops. CHANGELOG.md
// records every save with a way to undo it, and the copy each save replaced
// lives beside it under versions, so a rollback is a file copy rather than a
// guess.
//
// Only names and one-line descriptions ride in the model's prompt. The body
// loads when a skill is used, so a hundred skills cost a hundred lines of
// context rather than a hundred procedures. Listing and matching are therefore
// forgiving: a folder they cannot read is passed over, because the prompt must
// never break. Loading and running are strict: a folder with a file missing or
// a step over the size limit is refused with the file named, because using a
// broken skill must fail loudly.
//
// A skill runs with no model call at all. Each step goes through the permission
// function first, the skill's permissions block having become standing
// approvals that expire at midnight, and a step that wants a website the block
// does not name is put to the user instead of run. A step that fails hands back
// what it expected and what it saw, and the loop decides whether the model is
// needed. A skill is born from a page of documentation, from a finished task's
// record, or from the fourth answer of an after-action review, and each of
// those is offered to the user before anything is written.
package skill
