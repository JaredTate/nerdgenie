# Repository Map

<!-- generated: scripts/repomap -->
<!-- repo-map-contract: v1 -->

This file is generated from the repository tree. Regenerate it with `make repo-map`.
The generator omits dependency folders, build outputs, test artifacts, and version-control internals.

## Roots

- `./` - The three living documents, the plain-words explanation COEUS.md, the license, the third-party notes, the Makefile.
- `cmd/coeus/` - The one binary and its subcommands.
- `internal/` - The Go packages, one job each; see ARCHITECTURE.md for the list.
- `worker/` - The TypeScript browser and desktop workers.
- `test/` - Functional tests and fixtures.
- `scripts/` - The installer, the repo-map generator, CI helpers.
- `docs/` - The design, the work plan, the comparison, the research, the references, the briefs.

## Tree

```text
.
.github/workflows/check.yml
.gitignore
ARCHITECTURE.md
CLAUDE.md
COEUS.md
LICENSE
Makefile
README.md
REPO_MAP.md
THIRD_PARTY.md
cmd/coeus/commands.go
cmd/coeus/commands_test.go
cmd/coeus/doc.go
cmd/coeus/main.go
cmd/coeus/main_test.go
cmd/coeus/version.go
docs/COEUS_PLAN.md
docs/DEPENDENCIES.md
docs/HARNESS_V2.md
docs/WORK_PLAN.md
docs/briefs/wave-0/0.1-ready-to-build.md
docs/html/build.ts
docs/html/coeus-plan.html
docs/html/coeus.html
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
docs/research/work-plan-final-review.md
docs/research/work-plan-review.md
go.mod
internal/contract/browser.go
internal/contract/channel.go
internal/contract/clock.go
internal/contract/command.go
internal/contract/config.go
internal/contract/config_test.go
internal/contract/contract_test.go
internal/contract/desktop.go
internal/contract/doc.go
internal/contract/exitcode.go
internal/contract/home.go
internal/contract/home_test.go
internal/contract/identifier.go
internal/contract/identifier_test.go
internal/contract/job.go
internal/contract/known_test.go
internal/contract/memory.go
internal/contract/model.go
internal/contract/permission.go
internal/contract/provider_test.go
internal/contract/record.go
internal/contract/sandbox.go
internal/contract/secrets.go
internal/contract/skill.go
internal/contract/socket.go
internal/contract/socket_test.go
internal/contract/store.go
internal/contract/tool.go
internal/contract/toolnames.go
internal/contract/usertool.go
internal/contract/usertool_test.go
internal/lint/comments.go
internal/lint/doc.go
internal/lint/errormessage.go
internal/lint/exported.go
internal/lint/length.go
internal/lint/lint.go
internal/lint/lint_test.go
internal/lint/names.go
internal/lint/rules_test.go
internal/lint/testdata/borrowedheader/bad.go
internal/lint/testdata/borrowedheader/doc.go
internal/lint/testdata/clean/clean.go
internal/lint/testdata/clean/doc.go
internal/lint/testdata/commentsentence/bad.go
internal/lint/testdata/commentsentence/doc.go
internal/lint/testdata/doccomment/bad.go
internal/lint/testdata/doccomment/doc.go
internal/lint/testdata/errormessage/bad.go
internal/lint/testdata/errormessage/doc.go
internal/lint/testdata/filelength/bad.go
internal/lint/testdata/filelength/doc.go
internal/lint/testdata/functionlength/bad.go
internal/lint/testdata/functionlength/doc.go
internal/lint/testdata/identifiername/bad.go
internal/lint/testdata/identifiername/doc.go
internal/lint/testdata/packagedoc/bad.go
internal/testkit/browser.go
internal/testkit/browser_test.go
internal/testkit/browsermore_test.go
internal/testkit/browserpages.go
internal/testkit/browserserver.go
internal/testkit/browserserver_test.go
internal/testkit/channel.go
internal/testkit/channel_test.go
internal/testkit/checks_catch_more_test.go
internal/testkit/checks_catch_test.go
internal/testkit/checks_channel.go
internal/testkit/checks_model.go
internal/testkit/checks_run.go
internal/testkit/checks_screen.go
internal/testkit/checks_state.go
internal/testkit/clock.go
internal/testkit/clock_test.go
internal/testkit/desktop.go
internal/testkit/desktop_test.go
internal/testkit/doc.go
internal/testkit/fortystep.go
internal/testkit/fortystep_test.go
internal/testkit/fortystepchecks.go
internal/testkit/golden.go
internal/testkit/golden_test.go
internal/testkit/home.go
internal/testkit/home_test.go
internal/testkit/integration_test.go
internal/testkit/job.go
internal/testkit/job_test.go
internal/testkit/memory.go
internal/testkit/memory_test.go
internal/testkit/model.go
internal/testkit/model_test.go
internal/testkit/permission.go
internal/testkit/permission_test.go
internal/testkit/providerserver.go
internal/testkit/providerserver_test.go
internal/testkit/providerstream.go
internal/testkit/sandbox.go
internal/testkit/sandbox_test.go
internal/testkit/searchserver.go
internal/testkit/searchserver_test.go
internal/testkit/secrets.go
internal/testkit/secrets_test.go
internal/testkit/signalcli.go
internal/testkit/signalcli_test.go
internal/testkit/skill.go
internal/testkit/skill_test.go
internal/testkit/store.go
internal/testkit/store_test.go
internal/testkit/testdata/greeting.txt
internal/testkit/tools.go
internal/testkit/tools_test.go
scripts/coverage.sh
scripts/fuzz.sh
scripts/repomap/doc.go
scripts/repomap/drift_test.go
scripts/repomap/generate.go
scripts/repomap/generate_test.go
scripts/repomap/main.go
scripts/repomap/main_test.go
scripts/stylecheck/doc.go
scripts/stylecheck/main.go
scripts/stylecheck/main_test.go
test/fixtures/forty-step/task.json
test/functional/sample_test.go
worker/browser/PROTOCOL.md
```
