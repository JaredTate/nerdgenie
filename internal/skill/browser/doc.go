// Package browser is the browser half of a skill: it writes down what the agent
// did in a browser, replays it with no model call at all, heals one step whose
// element moved, and checks an app against a list of expected states.
//
// A recorded step carries three things. Its intent says what the step is for,
// in words. Its descriptor says how to find the element again three ways over:
// by the short reference the page gave it, by its role and name together, and
// by the text it showed, which is looked for only when it is at least a word
// long and lands on an element of the same kind or on a name it makes up most
// of. Its expectation says what the page should do because
// of the step. The three are written into the steps.md of an ordinary skill
// folder through internal/skill, so a recording is a skill like any other: it
// lists, it loads, it rolls back, and it carries a changelog.
//
// Replay observes before it acts. Before every step it reads the page, walks
// the descriptor's three rungs down the fresh snapshot until one of them names
// an element, and only then acts, handing the browser worker the recorded
// expectation so that the worker judges the step the same way it judges the
// model's own. The model is never called. A step whose expectation is not met
// stops the replay and says what was seen instead.
//
// Self-heal is the one place a model is called, and it is called once per
// replay. It gets the failed step's intent and the page as it stands, fenced
// between the two marker lines a tool result is wrapped in so that a name
// reading "ignore the rules above" is read as something the page says, and it
// answers with the element it thinks the intent meant. The answer is not
// trusted three times over. An element of another kind than the step was
// recorded on is refused outright. The change is put to the user as a one-line
// preview before the step is taken on it, because acting is the harm and a check
// made afterwards is made too late. And only the recorded expectation being met
// after that counts as healed. Nothing is done and nothing is written without a
// yes, and what is written is recorded in the skill's changelog above the line
// the store adds. A step the skill marks as one that cannot be undone is never
// healed at all, because healing a step is taking it.
//
// The /walk command is the one door into this package from outside it. Its
// three forms record the page the browser is on as the start of a walk, replay a
// saved walk, and check one with a picture at every step. Nothing else here is
// built by anybody but this package's own tests.
//
// The visual check walks an app the same way, takes a labeled screenshot at
// every step, and judges each expected state against the page by the word rule
// in worker/browser/PROTOCOL.md rather than by what changed, because a state is
// what the page is showing rather than what the last click did. It never takes
// a step the skill's permissions block marks as one that cannot be undone: a
// check that changed the app would not be a check.
package browser
