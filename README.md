# repoanalyzer

Terminal dashboard of stats and charts for the git repo you run it in.

## Install

Needs Go 1.24+ (`brew install go`). Without cloning:

```sh
go install github.com/mynameisgump/repoanalyzer/cmd/repoanalyzer@latest
```

That puts it in `$(go env GOPATH)/bin` (usually `~/go/bin`), which needs to be on your `PATH`.

Or from a clone of this repo:

```sh
./install.sh                         # builds and installs to ~/.local/bin
./install.sh --dir /usr/local/bin    # somewhere else (uses sudo if it has to)
./install.sh --uninstall
```

The script tells you if the install directory isn't on your `PATH` yet. For pull request stats, also
install the GitHub CLI (`brew install gh && gh auth login`); without it the PR numbers are estimated from commit messages.

## Usage

```sh
cd some/repo && repoanalyzer
repoanalyzer ~/code/app ~/code/api      # compare up to 4 repos
```

| Flag | |
|---|---|
| `repo ...` | repos to analyze; more than one compares them (default: current directory) |
| `-C dir` | analyze a repo other than the current directory |
| `--since "6 months ago"` | limit history stats (any `git log --since` value) |
| `--json` | print everything as JSON instead of opening the dashboard (an array when comparing) |
| `--no-remote` | skip GitHub; PR counts fall back to an estimate from commit messages |
| `--all-files` | count data, config and docs too (JSON, CSV, YAML, Markdown, Godot scenes, …); by default only code counts |
| `--no-mouse` | don't capture the mouse, so dragging selects text as usual |

Keys: `←/→` or `1`–`5` switch tabs, `↑/↓`/`j/k` scroll, `t` cycles the time window, `o` toggles overlap and `c` side-by-side columns (when comparing),
`+`/`-` make the Activity charts taller or shorter, `f` filters the Files lists to non-test or test files, `e` shows every row of the current tab's lists
(wherever you see "+ N more"; press again to show fewer), `q` quits.

Mouse: the wheel scrolls, and clicking a tab, a footer hint, a setting in the header or a "+ N more" note does what its key does; anything clickable lights up under the pointer.
While the dashboard has the mouse, hold Shift (Option in iTerm2) to select text, or run with `--no-mouse`.

## Time window

`t` cycles all time → last year → last 6 months → last 3 months (→ overlap when comparing).
`o` jumps straight to **overlap** – from the latest first commit to the earliest last commit across the
compared repos, so their charts cover the same dates – and pressing it again goes back to the previous window.
The active window shows at the right of the tab bar.

Everything built from history follows the window: commits, contributors, authors, the Activity charts and heatmap,
PR sizes, test shares and most-changed files. Lines of code, languages and largest files describe the current
tree and say so; GitHub's PR numbers always cover all time. Unlike `--since`, switching windows doesn't re-read git:
stats are recomputed from the history already loaded.

## Tabs

- **Overview** – lines of code, files, commits, contributors, repo age, merged PRs, languages, weekly commit sparkline
- **Authors** – contributors by commits with share, lines added/removed, first/last commit, and average PR size (lines changed) per author
- **Activity** – total lines over time, commits per week, average PR size per month, the share of each month's commits
  that changed a test file, and a day × hour heatmap.
  `+`/`-` resize the line charts in 4-row steps (6 rows up to a full screen); taller charts show finer detail, not just bigger lines
- **Files** – most frequently changed files and the largest files, with test files tagged. `f` cycles all files →
  non-test files → test files only
- **Pull Requests** – open/merged/closed counts, median time to merge, top PR authors (via `gh`)

## Comparing repos

Each repo gets its own color. The Activity tab overlays every repo on the same line charts
(with a legend) and puts the heatmaps side by side; the Overview tab becomes a metric-by-repo
table. Authors, Files and Pull Requests show one section per repo, stacked; `c` puts them side by side in columns
instead (also the Overview's language lists). Side by side, the repos' Authors tables always show the same columns.
When the terminal is too narrow for that (columns under about 50 characters, or Authors tables that would have to be
squeezed), they stay stacked and the header says so.

## How it works

- `internal/gitlog` reads history in **one** `git log --numstat` pass; every history stat comes from that.
  Lines over time uses a second `--first-parent` pass so changes are dated by when they landed on the branch, then walks backwards from today's count.
  The same pass gives PR sizes: a PR merge commit's first-parent diff (or a squash commit's diff) is the whole PR.
  Squash commits are credited to their author; merge commits to the author of the PR branch's last commit, since the merge itself is authored by whoever clicked merge.
- `internal/loc` counts lines in `git ls-files` in parallel, skipping binaries, lockfiles and files >4MB.
  Unless `--all-files` is given, files that aren't in a programming language (data, config, docs, assets like Godot
  `.tscn`/`.tres`, and unrecognized extensions) are left out of every stat: line counts, languages, file lists,
  lines added/deleted, PR sizes and lines over time. Commits that only touch them still count as commits.
  Test files are recognized by path and name (`tests/`, `__tests__/`, `spec/`, `foo_test.go`, `foo.test.ts`, `test_foo.py`, `FooTest.java`, …);
  tests written inside source files, like Rust's `#[cfg(test)]`, aren't.
- `internal/stats` holds the pure aggregation logic (unit tested).
- `internal/remote` queries GitHub's GraphQL API through `gh api`, so it uses your existing `gh auth login`.
- `internal/ui` is the Bubble Tea dashboard; local stats show first and PR stats fill in when they arrive.

Contributors are merged by name and respect `.mailmap`; add one to a repo to merge identities further.
