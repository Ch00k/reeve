# reeve

A terminal UI listing the Claude Code sessions running on this machine, interactive and background, with a way to stop
them gracefully.

For each session it shows the kind, state, name, working directory, title and how long it has been running, sorted by any
of those, the youngest session first by default. The columns fit the terminal's width and follow it when the window or
tmux pane is resized. A floating window shows the selected session's last few messages, Claude's replies rendered as
Markdown in the style Claude Code itself uses. The list refreshes every 3 seconds.

## Installation

On Linux or macOS, amd64 or arm64, download the latest release binary into the first writable directory on `PATH` out of
`~/.local/bin`, `~/bin` and `/usr/local/bin`:

```
curl -fsSL https://raw.githubusercontent.com/Ch00k/reeve/main/install.sh | sh
```

The script checks the binary against the release's `SHA256SUMS`. Running it again upgrades. Alternatively, with Go
installed:

```
go install github.com/Ch00k/reeve@latest
```

## Usage

```
reeve [-messages N]
```

`-messages` sets how many of a session's last messages the messages window shows (3 by default). `-version` prints the
version.

| Key                       | Action                                                     |
| ------------------------- | ---------------------------------------------------------- |
| up/down, j/k, pgup/pgdown | select a session                                           |
| o                         | sort by the next column                                    |
| O                         | reverse the sort order                                     |
| m                         | open the selected session's messages window                |
| t                         | go to the selected session's tmux pane (only inside tmux)  |
| s                         | ask to stop the selected session                           |
| y/enter                   | confirm                                                    |
| n/esc                     | cancel                                                     |
| r                         | refresh now                                                |
| q, ctrl+c                 | quit                                                       |

In the messages window, which opens scrolled to the end:

| Key                         | Action                 |
| --------------------------- | ---------------------- |
| up/down, j/k                | scroll a line          |
| pgup/pgdown, b/f, space     | scroll a page          |
| u/d                         | scroll half a page     |
| home/end, g/G               | go to the top / bottom |
| m, esc                      | close the window       |

## How it works

- Sessions come from `claude agents --json`, so `claude` must be on `PATH`.
- The title and last messages are read from the end of the session's transcript,
  `~/.claude/projects/*/<session id>.jsonl` (`$CLAUDE_CONFIG_DIR/projects` when that is set). The title is the latest
  one Claude Code generated for the conversation (its `ai-title` records), the same one the Mac notifications show; a
  new session has none until Claude Code writes the first. Tool calls and results, subagent messages and
  harness-injected messages (slash-command output, task notifications, reminders) are not counted as messages. A
  session whose transcript has been deleted shows no title or messages.
- The age is the time since the session started, as `claude agents` reports it.
- An interactive session's tmux pane is the one Claude Code recorded in `~/.claude/sessions/<pid>.json` when it started,
  shown with its current location when the tmux server still has that pane. When the app itself runs in tmux, going to
  it runs `tmux switch-client -t <pane>`. Background sessions have no pane; `claude attach <id>` opens one.
- Stopping a background session runs `claude stop <id>`; its conversation stays resumable.
- `claude stop` and `claude rm` confirm through Claude Code's background service but do not start it, and the service
  runs only while something needs it. So when a background session's process has died while its record still lists it as
  running, the service is down and both commands fail with "the background service may be restarting". When the service
  is down, the app starts it (`claude daemon run`) for the command and shuts it down afterwards with
  `claude daemon stop --any --keep-workers`, which leaves any background session started meanwhile running.
- When `claude stop` still fails on a background session, the app shows the error and offers to remove the session
  instead with `claude rm <id>`. That deletes the session and its worktree, so it can't be resumed; Claude Code refuses
  when the worktree holds unpushed or uncommitted work, and the refusal is shown.
- Stopping an interactive session sends SIGTERM to its process, on which Claude Code cleans up and exits. The session
  list is fetched again first, and the signal is sent only if that pid still belongs to the same session.

## Development

```
make build    # bin/reeve, a static binary
make test
make lint     # golangci-lint, fixing what it can
make install  # copies bin/reeve to ~/.local/bin
```

Cross-compile for a Mac with `GOOS=darwin GOARCH=arm64 make build`. `pre-commit install` sets up the hooks in
`.pre-commit-config.yaml`.

CI (`.github/workflows/ci.yml`) lints and runs the tests on Linux and macOS for every push to `main` and every pull
request.

## Releasing

```
make release-patch  # or release-minor, release-major
```

This tags the next `vX.Y.Z` on `main` and pushes it. CI then builds the binaries for Linux and macOS on amd64 and arm64
(`make dist`) and publishes them, with their `SHA256SUMS`, as a GitHub release. The binary's version is the tag, which
Go stamps into it at build time; a build of an untagged commit reports a pseudo-version.
