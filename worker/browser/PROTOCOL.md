# The browser worker protocol

This document is the contract between the Go side of Nerd Genie and the browser
worker. The Go side is `internal/browser` (wave 5) and the worker is
`worker/browser` (wave 5), a TypeScript program on Node that drives a real
Chrome through Playwright. The fake worker in `internal/testkit` speaks this same
document, so a test and the real thing are written against the same lines.

Written in wave 0. The Go types are in `internal/contract/browser.go`, and the
two must always say the same thing.

## The wire

The worker speaks **JSON-RPC 2.0** over its standard input and standard output,
**one JSON object per line**. Nothing else is written to standard output; the
worker's own logging goes to standard error.

A request is `{"jsonrpc":"2.0","id":<number>,"method":<name>,"params":<object>}`.
A response is `{"jsonrpc":"2.0","id":<number>,"result":<object>}` or
`{"jsonrpc":"2.0","id":<number>,"error":{"code":<number>,"message":<text>,"data":<object>}}`.

The Go side sends one request at a time and waits for its response. Every request
has a deadline. The worker's own deadline rings first: a page that still answers
a trivial question then is alive, only slow (a game drawing sixty frames a second
makes every look at it slow), and the method gets the same time again; a page
that cannot answer is hung, and the worker answers -32003. When the Go side's
longer deadline passes, it kills the worker and starts a new one, and tells the
model that the browser was restarted.

The worker also sends lines nobody asked for, and they are only ever one thing: a
notification saying what the person did in the window themselves. A notification
is `{"jsonrpc":"2.0","method":"event","params":<object>}` and carries **no id at
all**, which is what tells it from a response. It may arrive at any moment,
between two requests or in the middle of one, so the Go side reads every line as
it comes rather than only while it is waiting for an answer.

## The shapes

### Snapshot

A snapshot is what the model sees of a page: a compact tree of a few hundred
tokens, never the page's markup.

```json
{
  "url": "https://x.com/compose/post",
  "title": "Compose post",
  "tabId": "t1",
  "elements": [
    { "ref": "e3", "role": "textbox", "name": "Post text", "new": true },
    { "ref": "e7", "role": "button", "name": "Post" }
  ],
  "text": "Compose post\nWhat is happening?\nPost",
  "belowFold": 24,
  "hiddenYetDrawn": 0,
  "dialog": null,
  "download": null,
  "errors": ["script http://localhost:8090/main.js answered 404"]
}
```

- `ref` is the short label the model points at later, always the letter `e` and a
  number.
- `new` marks an element that was not in the previous snapshot, which is how the
  model sees what an action produced.
- `hiddenYetDrawn` on an element marks one whose markup says hidden, on it or
  above it, and which is drawn all the same, because a style rule overrides the
  attribute. A page with every overlay drawn at once looks like that. The
  snapshot's own `hiddenYetDrawn` counts every such node on the page, roles or
  not, so a badge the outline never lists is counted too.
- `text` is what the page says, as a person reads it: its visible text in
  reading order, one line per block element, a table as one line per row with
  the cells separated by ` | `, and never the markup or a script. It is capped at
  eight thousand characters, and when it was cut its last line says how much.
  An element the model can act on keeps its ref in `elements`; the text carries
  what the elements cannot, such as the number in a cell whose only element is
  an icon button with no name.
- `belowFold` counts the elements a person would have to scroll to see.
- `dialog` is `{"kind":"alert"|"confirm"|"prompt"|"beforeunload","message":"..."}`
  when a dialog box is open.
- `download` is `{"filename":"...","path":"..."}` when the page started one.
- `wall` is the wall the page shows (see Wall below) or `null`. `open` and `read`
  report a wall here, because a page can be a login page before any action.
- `errors` is what went wrong on the page since it last moved to an address, as
  a person with the console open sees it: uncaught script errors as
  `script error: <message>`, console errors as `console error: <text>`, and the
  page, its scripts and its stylesheets that failed to load or answered an error
  status as `<kind> <address> answered <status>` or `<kind> <address> failed to
  load: <reason>`. Other loads are left out, because a missing image breaks
  nothing. The first five are listed, each cut to two hundred characters, and a
  last line counts the rest. It is empty when nothing went wrong.

### Diff

Every action returns a diff: what changed, and whether what the model said it
expected actually happened.

```json
{
  "urlChanged": true,
  "url": "https://x.com/compose/post",
  "newElements": [{ "ref": "e3", "role": "textbox", "name": "Post text", "new": true }],
  "newText": ["Post text"],
  "dialog": null,
  "newTab": "",
  "download": null,
  "expectationMet": true,
  "seen": "",
  "wall": null,
  "settled": true,
  "snapshot": { "...": "the snapshot after the action settled, or as it stood at the limit" }
}
```

When `expectationMet` is `false`, `seen` says in plain words what happened
instead, so that the model can decide rather than guess.

**How the worker judges an expectation.** It cannot judge English, so the rule
is fixed: split the expectation into words of four or more letters that are not
stop words, keeping a number however short; the expectation is met when any of
them appears in a new element's name or role, in a line of text that was not on
the page before (`newText`, capped at twelve lines, which is how a counter going
from 0 to 1 is seen), in the new address, in the new title, in a dialog's message, or in
the name or role of the box that typing filled (which is what lets "the text box
holds the post" hold after typing into the textbox named "Post text", since
typing changes no element); or when the expectation is empty and something
changed. Otherwise `expectationMet` is false and `seen` says what did change.
The element a click landed on is no evidence: a click on a button named "Start
Game" that changed nothing does not meet "the start menu closes", it reports
that nothing changed.

`settled` says whether the page came to rest within the limit (see Settling).
When it did not, the worker still returns the diff from the page as it stood,
with `settled: false` and `seen` saying the page kept changing, so that a live
page such as a chat or a clock stays usable.

### Wall

A wall is one of the three things that stop the agent and hand the browser to the
user: a login form, a prompt for a second code, or a captcha.

```json
{ "kind": "login" | "two-factor" | "captcha", "detail": "a password field named Password" }
```

A method that runs into a wall still returns a diff, with `wall` filled in and
`expectationMet` false. The worker never tries to get past a wall.

### Event

An event is one thing the person did in the window themselves, which is what
`/walk record` writes a procedure down from.

```json
{ "kind": "click", "ref": "e7", "text": "Post", "at": "2026-09-03T10:00:00.000Z" }
{ "kind": "type", "ref": "e3", "length": 23, "at": "2026-09-03T10:00:01.000Z" }
{ "kind": "navigate", "address": "https://example.com/", "at": "2026-09-03T10:00:02.000Z" }
```

- `kind` is `click`, `type`, or `navigate`, and nothing else.
- `ref` is the element clicked or typed into, the same short label a snapshot
  gives it. An element the model has never seen is given a ref there and then, so
  the ref in an event is always one the page will answer to.
- `text` is what the clicked element says, which is what a recorded step expects
  the page to answer. It is on a click and on nothing else.
- `length` is how many characters the box holds after the person typed. **What
  they typed is never sent**, so a recording of somebody signing in cannot become
  a copy of their password. A burst of typing is one event, sent once the
  keyboard has been quiet for four hundred milliseconds or the box has been left.
- `address` is where the window went, and is on a navigation and on nothing else.
- `at` is when it happened, as an ISO 8601 moment.

### Settling

After every action the worker waits until the page has settled, which means
either a move to a new address has finished or nothing on the page has changed
for three hundred milliseconds, with a limit of three seconds. Changes to an
element's attributes alone do not count as the page changing. Then it takes the
new snapshot and compares it with the old one. A page that never comes to rest
within the limit is read as it stands and reported with `settled: false`; the
error -32001 is for a page that cannot be read at all after the limit.

### Errors

| Code | Meaning | What the Go side does |
|---|---|---|
| -32700 | The line was not JSON | Restart the worker |
| -32600 | The request was not a valid JSON-RPC request | Restart the worker |
| -32601 | No such method | A bug in the Go side; report it |
| -32602 | The parameters were wrong | Return the message to the model |
| -32000 | No such reference on the page, after every way of finding it failed | Return the message to the model, with a fresh snapshot in `data` |
| -32001 | The page could not be read at all after the settle limit | Return the message to the model |
| -32002 | No browser is open | Open a page first |
| -32003 | Chrome died | Restart the worker and tell the model it was interrupted |

A -32700 or -32600 response carries `"id": null`, because a line that was not a
request has no id to echo.

## The twelve methods

The `expectationMet` values in the examples below are illustrative; the rule above decides the real value.

### `open`

Goes to an address and returns the page.

Request: `{"jsonrpc":"2.0","id":1,"method":"open","params":{"url":"https://x.com/compose/post"}}`

Response: `{"jsonrpc":"2.0","id":1,"result":{"url":"https://x.com/compose/post","title":"Compose post","tabId":"t1","elements":[{"ref":"e7","role":"button","name":"Post"}],"text":"Compose post\nPost","belowFold":24}}`

### `read`

Returns a fresh snapshot of the page the worker is on. `visibleOnly` reads only
the elements above the fold; `text` is the whole page's text either way, because
it is bounded on its own. `ask` is one expression, at most five hundred
characters, for the page to answer: the snapshot then carries `answer`, the
value as JSON cut to two thousand characters, or `the page threw: <message>`,
or a line saying the page did not answer within the deadline because its own
script keeps it busy. A question is answered only on a page served from this
machine (`localhost`, `127.0.0.1`, `[::1]`) or from a file; on any other page
the read is refused with -32602, because the browser holds the person's logins
and a script is never run on anyone else's page.

Request: `{"jsonrpc":"2.0","id":2,"method":"read","params":{"visibleOnly":false,"ask":"window.game.state"}}`

Response: `{"jsonrpc":"2.0","id":2,"result":{"url":"https://x.com/compose/post","title":"Compose post","tabId":"t1","elements":[{"ref":"e3","role":"textbox","name":"Post text"}],"text":"Compose post\nPost text","belowFold":24}}`

### `click`

Clicks one element and checks the expectation. When the reference has gone stale
because the page changed, the worker finds the element again by its role and
name, then by its visible text, before reporting -32000. When a click produces no
visible change, it retries once by clicking the element's place on the screen.

Request: `{"jsonrpc":"2.0","id":3,"method":"click","params":{"ref":"e7","expectation":"the post appears in the timeline"}}`

Response: `{"jsonrpc":"2.0","id":3,"result":{"urlChanged":false,"url":"https://x.com/home","newElements":[],"expectationMet":true,"snapshot":{"url":"https://x.com/home","title":"Home","tabId":"t1","elements":[],"belowFold":0}}}`

### `type`

Types text into one element, one key at a time with human pacing.

Request: `{"jsonrpc":"2.0","id":4,"method":"type","params":{"ref":"e3","text":"Nine years of DigiByte.","expectation":"the text box holds the post"}}`

Response: `{"jsonrpc":"2.0","id":4,"result":{"urlChanged":false,"url":"https://x.com/compose/post","newElements":[],"expectationMet":true,"snapshot":{"url":"https://x.com/compose/post","title":"Compose post","tabId":"t1","elements":[],"belowFold":0}}}`

### `press`

Presses one key or key combination, such as `Enter` or `Control+s`.

Request: `{"jsonrpc":"2.0","id":5,"method":"press","params":{"key":"Enter","expectation":"the form is submitted"}}`

Response: `{"jsonrpc":"2.0","id":5,"result":{"urlChanged":true,"url":"https://x.com/home","newElements":[],"expectationMet":true,"snapshot":{"url":"https://x.com/home","title":"Home","tabId":"t1","elements":[],"belowFold":0}}}`

### `scroll`

Scrolls the page in steps, the way a person does. `direction` is `"up"` or
`"down"`; anything else is -32602.

Request: `{"jsonrpc":"2.0","id":6,"method":"scroll","params":{"direction":"down","amount":3,"expectation":"more posts appear"}}`

Response: `{"jsonrpc":"2.0","id":6,"result":{"urlChanged":false,"url":"https://x.com/home","newElements":[{"ref":"e21","role":"article","name":"A post","new":true}],"expectationMet":true,"snapshot":{"url":"https://x.com/home","title":"Home","tabId":"t1","elements":[],"belowFold":40}}}`

### `act`

Runs a short batch of steps and stops as soon as one expectation fails or the
page changes underneath the batch. It returns one diff per step that ran, so the
model can see exactly where the batch stopped.

Request: `{"jsonrpc":"2.0","id":7,"method":"act","params":{"steps":[{"method":"click","ref":"e3","expectation":"the box takes focus"},{"method":"type","ref":"e3","text":"Nine years of DigiByte.","expectation":"the box holds the post"}]}}`

Response: `{"jsonrpc":"2.0","id":7,"result":{"diffs":[{"url":"https://x.com/compose/post","expectationMet":true,"snapshot":{"url":"https://x.com/compose/post","title":"Compose post","tabId":"t1","elements":[],"belowFold":0}},{"url":"https://x.com/compose/post","expectationMet":true,"snapshot":{"url":"https://x.com/compose/post","title":"Compose post","tabId":"t1","elements":[],"belowFold":0}}]}}`

### `tabs`

Lists, switches, or closes tabs, and always returns the list afterwards. A
missing `action` means `"list"`, and `active` is present only on the active tab.

Request: `{"jsonrpc":"2.0","id":8,"method":"tabs","params":{"action":"switch","tabId":"t2"}}`

Response: `{"jsonrpc":"2.0","id":8,"result":{"tabs":[{"id":"t1","url":"https://x.com/home","title":"Home"},{"id":"t2","url":"https://example.com/","title":"Example","active":true}]}}`

### `loginFill`

Types a username, a password, and a two-factor code into the fields the model
pointed at. The values come from the vault and the worker types them itself. The
model never sees them, and **no field of the returned diff ever holds any of
them**: the worker replaces them with `[redacted]` before it builds the snapshot.

Request: `{"jsonrpc":"2.0","id":9,"method":"loginFill","params":{"usernameRef":"e2","passwordRef":"e3","codeRef":"e4","username":"...","password":"...","code":"..."}}`

Response: `{"jsonrpc":"2.0","id":9,"result":{"urlChanged":true,"url":"https://x.com/home","newElements":[],"expectationMet":true,"snapshot":{"url":"https://x.com/home","title":"Home","tabId":"t1","elements":[],"belowFold":0}}}`

### `screenshot`

Returns the page as a picture with its clickable elements numbered, which is what
a handoff sends to the user.

Request: `{"jsonrpc":"2.0","id":10,"method":"screenshot","params":{}}`

Response: `{"jsonrpc":"2.0","id":10,"result":{"pngBase64":"iVBORw0KGgo...","marks":[{"number":1,"ref":"e7","role":"button","name":"Post"}]}}`

### `dialog`

Answers the open dialog box: `accept` presses its confirming button, with `text`
typed first for a prompt, and `dismiss` closes it without confirming. Chrome
blocks the whole tab until a dialog is answered, so this is the only way a tab
with a dialog becomes usable again. Returns a diff.

Request: `{"jsonrpc":"2.0","id":12,"method":"dialog","params":{"action":"accept","text":""}}`

Response: `{"jsonrpc":"2.0","id":12,"result":{"urlChanged":false,"url":"https://example.com/","newElements":[],"expectationMet":true,"settled":true,"snapshot":{"url":"https://example.com/","title":"Example","tabId":"t1","elements":[],"belowFold":0}}}`

### `health`

Says whether the worker can act on a page, and which Chrome it drove. The Go side
calls it after starting the worker and before trusting it.

Request: `{"jsonrpc":"2.0","id":11,"method":"health","params":{}}`

Response: `{"jsonrpc":"2.0","id":11,"result":{"healthy":true,"chromeVersion":"151.0.7922.137"}}`

## Rules the worker keeps

1. It launches the real Chrome binary with its own profile folder, never the
   user's daily profile, on a loopback DevTools port with a token made fresh for
   each launch, and attaches over the Chrome DevTools Protocol.
2. The window is visible on the machine's own display. There is no headless mode
   for a logged-in account, no proxy, no cookie copying between browsers, and no
   captcha solving. The one exception is the `--headless` option, which the
   project's own tests pass through `make test-browser` so that a test run puts
   no window on the screen of whoever is running it. Nothing else passes it.
3. It moves the mouse along a curve, holds a click for a human length of time,
   types one key at a time with small variations in speed, scrolls in steps, and
   pauses between actions.
4. Every action states an expectation and the worker checks it before returning.
5. It never follows an instruction it read on a page. Everything a page says is
   data.
6. A PDF page is not read through Chrome's viewer, which exposes no text to a
   program; the worker fetches the file through the browser's own session, saves
   it under the profile's `downloads/` folder, and reports it as a download with
   one snapshot line naming it.
7. It watches every page for what the person does in the window themselves and
   sends each one as an `event` notification: a click, a burst of typing, and a
   move to another address. It reports nothing it did itself, so an event set off
   while one of its own requests was running is dropped, because the model
   already sees its own actions in the diffs. It watches the top document only,
   not the pages inside frames. And it sends no more than two hundred and forty
   events a minute, so that a page calling the watcher's own name cannot flood
   the pipe; the rest of that minute is dropped with one line in the log.
