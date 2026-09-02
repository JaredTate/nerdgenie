# Third-party designs and files

Coeus ports designs from these projects. Porting means a worker read the reference file, understood the idea, and wrote it fresh in Go. No code was copied. Each Go file that ports a design names the project and the reference path in a comment at its top.

| Project | License | What was ported |
|---|---|---|
| OpenClaw | MIT | The loop with hooks, tool-call repair, the loop detector, the retry refund, the cache boundary, the Signal client, the cron backoff, the pinned DNS lookup, the external-content wrapper, the delivery queue, the pairing store, the boot check, the browser launch and attach |
| Hermes Agent | MIT | The prompt tiers, verbatim user messages, the memory caps, session search, pairing, the reliability files, the sudo prompt, cron incidents and notepad and monitor, the Signal platform, the skill ledger, the learn prompt |
| Prime Agent | MIT | The provider types, overflow detection, goals, budgets, refinement with rollback, cron as heartbeat |
| OpenCode | MIT | The permission engine, the tool registry and descriptions, the file tools, truncation, invalid calls, the retry policy, the command registry, the terminal rules, the skill loader |
| ZeroClaw | MIT or Apache-2.0 | The channel trait, the Signal channel, the sender allowlist, the cron store, the tool-call parser, the turn cap, the Landlock wrapper, the updater, the skill creator, the encrypted secrets |
| Moltis | MIT | The Chrome manager with a persistent profile, the snapshot with numbered refs |
| browser-use | MIT | The page serializer, the per-step evaluation fields |
| Codex CLI | Apache-2.0 | The shell tool's justification field, the Landlock rules |
| HomeRecon | private, same owner | The three living documents and the repo-map drift test |

**Ported so far.** As of wave 0: HomeRecon's repository-map generator and its
contract test, ported fresh into Go in `scripts/repomap/`; the two provider wire
protocols read from Prime's `packages/ai/src/providers/` and written fresh as the
fake provider server in `internal/testkit/providerserver.go`; and the three
signal-cli HTTP paths and the event envelope read from OpenClaw's
`extensions/signal/src/` and written fresh as the fake daemon in
`internal/testkit/signalcli.go`. Each of those Go files names its reference path
in a comment at the top, which the style checker enforces.

## Verbatim copies in `docs/reference/`

These files are unmodified copies kept for reading, because their projects were cloned only during research and are not on the development machine. They remain under their own licenses, named above, and are not part of the Coeus program.

- `docs/reference/browser-use/` — `serializer.py`, `prompts.py`, `views.py` (MIT)
- `docs/reference/moltis/` — `manager.rs`, `snapshot.rs`, `types.rs` (MIT)
- `docs/reference/codex/` — `shell_spec.rs`, `landlock.rs` (Apache-2.0)
