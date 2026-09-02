# Repository Map

<!-- generated: scripts/repo-map.sh -->
<!-- repo-map-contract: v1 -->

This file is generated from the repository tree. Regenerate it with `make repo-map`.
The generator omits dependency folders, build outputs, test artifacts, and version-control internals.

## Roots

- `./` - The three living documents, the license, the third-party notes, the Makefile.
- `cmd/coeus/` - The one binary and its subcommands.
- `internal/` - The Go packages, one job each; see ARCHITECTURE.md for the list.
- `worker/` - The TypeScript browser and desktop workers.
- `test/` - Functional tests and fixtures.
- `scripts/` - The installer, the repo-map generator, CI helpers.
- `docs/` - The design, the work plan, the comparison, the research, the references, the briefs.

## Tree

```text
.
ARCHITECTURE.md
CLAUDE.md
docs/COEUS_PLAN.md
docs/HARNESS_V2.md
docs/html/build.ts
docs/html/coeus-plan.html
docs/html/harness-v2.html
docs/html/work-plan.html
docs/reference/browser-use/prompts.py
docs/reference/browser-use/serializer.py
docs/reference/browser-use/views.py
docs/reference/codex/landlock.rs
docs/reference/codex/shell_spec.rs
docs/reference/moltis/manager.rs
docs/reference/moltis/snapshot.rs
docs/reference/moltis/types.rs
docs/research/00-notes.md
docs/research/00-rosie-live-measurements.md
docs/research/01-openclaw-architecture.md
docs/research/02-openclaw-failure-modes.md
docs/research/03-hermes.md
docs/research/04-prime-agent.md
docs/research/05-opencode.md
docs/research/06-atomic-and-field-survey.md
docs/research/07-news-since-august.md
docs/research/08-language-runtime.md
docs/research/09-security-reliability.md
docs/research/10-metrics.md
docs/research/11-browser-desktop-skills.md
docs/research/12-loop-comparison.md
docs/research/13-zeroclaw-moltis.md
docs/research/14-doom-loop-and-tokens.md
docs/research/15-tool-inventory.md
docs/research/16-browser-agent-spec.md
docs/research/17-chatgpt-state-conversation.txt
docs/research/coeus-plan-review.md
docs/research/harness-v2-review.md
docs/research/work-plan-review.md
docs/WORK_PLAN.md
README.md
REPO_MAP.md
scripts/repo-map.sh
THIRD_PARTY.md
```
