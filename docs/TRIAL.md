# The human trial: what to run, in order

This is the checklist from `docs/WORK_PLAN.md` written as commands, for the person sitting at the development machine. Every trial runs on the local Qwen 3.8 through the llama-server daemon on port 19091, which is the `local` alias a fresh install ships with. Check it first:

```
curl -s http://127.0.0.1:19091/health
```

It must answer `{"status":"ok"}`. If it does not, start it with the command in `CLAUDE.md` and wait for that answer.

## 1. Install on a clean account, run init, get to the first reply

From the repository, build a release and install it into a throwaway home so nothing touches your own:

```
make release
export HOME=/tmp/nerdgenie-clean && mkdir -p $HOME
sh scripts/install.sh --from "$(ls dist/nerdgenie-*-amd64.tar.gz)" --no-signal -- --yes --model local --signal off
export PATH="$HOME/.local/bin:$PATH" && nerdgenie doctor
```

Then, in one terminal, `nerdgenie serve`; in another, `nerdgenie`. Type a question. The first frame must be there at once, the reply must stream, and nothing should need a document. Put your `HOME` back afterwards.

The sandbox is off on a fresh install, and the first line `nerdgenie serve` prints says so: `the sandbox is off: commands run straight on this machine as you, and the ask-me-first list is the gate`. `nerdgenie doctor` reports the same under `the sandbox setting`. To try the fence, change the `sandbox = "off"` line that `init` wrote in `config.toml` to `sandbox = "fence"` (the comment above it says what the fence does), start `serve` again, and ask the agent to read a file in `/tmp`: the command is refused with the fence on and reads the file with it off. Any other word on that line stops the agent at start with both values named.

For the trials below, the repository build is enough: `make build`, then `NERDGENIE_HOME=/tmp/nerdgenie-trial/.nerdgenie bin/nerdgenie serve` in one window and `NERDGENIE_HOME=/tmp/nerdgenie-trial/.nerdgenie bin/nerdgenie` in another.

## 2. The terminal screen

Watch for: the first frame at once and at the terminal's real size; streaming visible while a reply is written; a preview card you answer with `a`, `A`, or `r` (ask it to write a file in the working folder to get one); a masked prompt that never echoes (`/vault add` asks for a secret); Escape stops a reply or a task; no flicker on resize; `/` opens the palette; `/tasks` and `/jobs` show the record.

## 3. Signal

```
nerdgenie signal link
```

Follow what it prints on your phone. Then, from the phone, send the number a message; the agent answers with a pairing code that you confirm with `/pair <code>` in the terminal. Have one conversation from the phone that needs a preview, and approve it from the phone.

## 4. The browser

Serve the fixture site in a third window:

```
go run ./scripts/fixturesite
```

It prints its address and the one credential it accepts. Store that credential with `/vault add fixture <address> 127.0.0.1 jared` (it asks for the password on a masked prompt), then ask the agent to sign in at the address and post "hello from nerdgenie" on the compose page. Watch the Chrome window: it must be visible, the pacing must look human, the post must be previewed before it is sent, and when you ask the agent to open the site's `/captcha` page it must hand the browser to you rather than guess.

## 5. Jobs, the desktop, and a bad release

A scheduled job: `/cron` lists them; ask the agent for "a job that writes the time to clock.txt in the working folder every two minutes" and watch `/jobs` and the file.

The desktop: ask the agent to "open the text editor and type hello". The computer tool asks before the first action on an application; approve it and watch.

A bad release rolled back: this one needs the real service, so install it first (`make install`, which runs `nerdgenie install` on your own home, then `systemctl --user status nerdgenie.service`). Then `scripts/trial/bad-release.sh` builds a good release and a bad one whose binary exits at once, and prints the two `nerdgenie update --from` commands. Run the good one, then the bad one, and watch the agent come back on the good version within sixty seconds with a line saying so. `nerdgenie uninstall` removes the service afterwards and keeps your home.

## Writing the notes

Everything confusing, slow, or ugly goes under the wave's section in `docs/PROGRESS.md`, in your words. The orchestrator turns each note into a brief.
