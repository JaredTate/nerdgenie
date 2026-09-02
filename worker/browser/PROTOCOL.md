# The browser worker protocol

This document is the contract between the Go side of Coeus and the browser
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
has a deadline; when it passes, the Go side kills the worker and starts a new
one, and tells the model that the browser was restarted.

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
  "belowFold": 24,
  "dialog": null,
  "download": null
}
```

- `ref` is the short label the model points at later, always the letter `e` and a
  number.
- `new` marks an element that was not in the previous snapshot, which is how the
  model sees what an action produced.
- `belowFold` counts the elements a person would have to scroll to see.
- `dialog` is `{"kind":"alert"|"confirm"|"prompt"|"beforeunload","message":"..."}`
  when a dialog box is open.
- `download` is `{"filename":"...","path":"..."}` when the page started one.

### Diff

Every action returns a diff: what changed, and whether what the model said it
expected actually happened.

```json
{
  "urlChanged": true,
  "url": "https://x.com/compose/post",
  "newElements": [{ "ref": "e3", "role": "textbox", "name": "Post text", "new": true }],
  "dialog": null,
  "newTab": "",
  "download": null,
  "expectationMet": true,
  "seen": "",
  "wall": null,
  "snapshot": { "...": "the snapshot after the action settled" }
}
```

When `expectationMet` is `false`, `seen` says in plain words what happened
instead, so that the model can decide rather than guess.

### Wall

A wall is one of the three things that stop the agent and hand the browser to the
user: a login form, a prompt for a second code, or a captcha.

```json
{ "kind": "login" | "two-factor" | "captcha", "detail": "a password field named Password" }
```

A method that runs into a wall still returns a diff, with `wall` filled in and
`expectationMet` false. The worker never tries to get past a wall.

### Settling

After every action the worker waits until the page has settled, which means
either a move to a new address has finished or nothing on the page has changed
for three hundred milliseconds, with a limit of three seconds. Then it takes the
new snapshot and compares it with the old one.

### Errors

| Code | Meaning | What the Go side does |
|---|---|---|
| -32700 | The line was not JSON | Restart the worker |
| -32600 | The request was not a valid JSON-RPC request | Restart the worker |
| -32601 | No such method | A bug in the Go side; report it |
| -32602 | The parameters were wrong | Return the message to the model |
| -32000 | No such reference on the page, after every way of finding it failed | Return the message to the model, with a fresh snapshot in `data` |
| -32001 | The page did not settle before the limit | Return the message to the model |
| -32002 | No browser is open | Open a page first |
| -32003 | Chrome died | Restart the worker and tell the model it was interrupted |

## The eleven methods

### `open`

Goes to an address and returns the page.

Request: `{"jsonrpc":"2.0","id":1,"method":"open","params":{"url":"https://x.com/compose/post"}}`

Response: `{"jsonrpc":"2.0","id":1,"result":{"url":"https://x.com/compose/post","title":"Compose post","tabId":"t1","elements":[{"ref":"e7","role":"button","name":"Post"}],"belowFold":24}}`

### `read`

Returns a fresh snapshot of the page the worker is on. `visibleOnly` reads only
what is above the fold.

Request: `{"jsonrpc":"2.0","id":2,"method":"read","params":{"visibleOnly":false}}`

Response: `{"jsonrpc":"2.0","id":2,"result":{"url":"https://x.com/compose/post","title":"Compose post","tabId":"t1","elements":[{"ref":"e3","role":"textbox","name":"Post text"}],"belowFold":24}}`

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

Scrolls the page in steps, the way a person does.

Request: `{"jsonrpc":"2.0","id":6,"method":"scroll","params":{"direction":"down","amount":3,"expectation":"more posts appear"}}`

Response: `{"jsonrpc":"2.0","id":6,"result":{"urlChanged":false,"url":"https://x.com/home","newElements":[{"ref":"e21","role":"article","name":"A post","new":true}],"expectationMet":true,"snapshot":{"url":"https://x.com/home","title":"Home","tabId":"t1","elements":[],"belowFold":40}}}`

### `act`

Runs a short batch of steps and stops as soon as one expectation fails or the
page changes underneath the batch. It returns one diff per step that ran, so the
model can see exactly where the batch stopped.

Request: `{"jsonrpc":"2.0","id":7,"method":"act","params":{"steps":[{"method":"click","ref":"e3","expectation":"the box takes focus"},{"method":"type","ref":"e3","text":"Nine years of DigiByte.","expectation":"the box holds the post"}]}}`

Response: `{"jsonrpc":"2.0","id":7,"result":{"diffs":[{"url":"https://x.com/compose/post","expectationMet":true,"snapshot":{"url":"https://x.com/compose/post","title":"Compose post","tabId":"t1","elements":[],"belowFold":0}},{"url":"https://x.com/compose/post","expectationMet":true,"snapshot":{"url":"https://x.com/compose/post","title":"Compose post","tabId":"t1","elements":[],"belowFold":0}}]}}`

### `tabs`

Lists, switches, or closes tabs, and always returns the list afterwards.

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
   captcha solving.
3. It moves the mouse along a curve, holds a click for a human length of time,
   types one key at a time with small variations in speed, scrolls in steps, and
   pauses between actions.
4. Every action states an expectation and the worker checks it before returning.
5. It never follows an instruction it read on a page. Everything a page says is
   data.
