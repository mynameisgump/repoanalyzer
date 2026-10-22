# repoanalyzer

Terminal dashboard of stats and charts for the git repo you run it in.

```sh
go build -o bin/repoanalyzer ./cmd/repoanalyzer   # or: go install ./cmd/repoanalyzer
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

Keys: `←/→` or `1`–`5` switch tabs, `↑/↓`/`j/k` scroll, `q` quits.

## Tabs

- **Overview** – lines of code, files, commits, contributors, repo age, merged PRs, languages, weekly commit sparkline
- **Authors** – contributors by commits with share, lines added/removed, first/last commit, and average PR size (lines changed) per author
- **Activity** – total lines over time, commits per week, average PR size per month, and a day × hour heatmap
- **Files** – most frequently changed files, the largest files, and how much of the code is tests (overall and per language)
- **Pull Requests** – open/merged/closed counts, median time to merge, top PR authors (via `gh`)

## Comparing repos

Each repo gets its own color. The Activity tab overlays every repo on the same line charts
(with a legend) and puts the heatmaps side by side; the Overview tab becomes a metric-by-repo
table. Authors, Files and Pull Requests show one section per repo.

## How it works

- `internal/gitlog` reads history in **one** `git log --numstat` pass; every history stat comes from that.
  Lines over time uses a second `--first-parent` pass so changes are dated by when they landed on the branch, then walks backwards from today's count.
  The same pass gives PR sizes: a PR merge commit's first-parent diff (or a squash commit's diff) is the whole PR.
  Squash commits are credited to their author; merge commits to the author of the PR branch's last commit, since the merge itself is authored by whoever clicked merge.
- `internal/loc` counts lines in `git ls-files` in parallel, skipping binaries, lockfiles and files >4MB.
  Test files are recognized by path and name (`tests/`, `__tests__/`, `spec/`, `foo_test.go`, `foo.test.ts`, `test_foo.py`, `FooTest.java`, …).
- `internal/stats` holds the pure aggregation logic (unit tested).
- `internal/remote` queries GitHub's GraphQL API through `gh api`, so it uses your existing `gh auth login`.
- `internal/ui` is the Bubble Tea dashboard; local stats show first and PR stats fill in when they arrive.

Contributors are merged by name and respect `.mailmap`; add one to a repo to merge identities further.
