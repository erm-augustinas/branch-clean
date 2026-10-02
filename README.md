# branch-clean

TUI that deletes local git branches (and their worktrees) which have no equivalent on the remote.

```sh
go build -o branch-clean . && cd /path/to/repo && /path/to/branch-clean
```

On start it runs `git fetch --prune --all`, then lists local branches whose upstream is gone
or that have no same-named remote branch. All are pre-selected.

| Key | Action |
| --- | --- |
| ↑/↓, j/k | move |
| space | toggle |
| a | select all / none |
| y | delete selected |
| n, q, esc | cancel |

Worktrees are removed first (`git worktree remove`), then the branch with `git branch -D`.
Worktrees with uncommitted changes are marked ⚠ and will fail to be removed (their branch is kept).
Never listed: the default branch, the branch/worktree you run it from, the main worktree, locked worktrees.
