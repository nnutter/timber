---
name: timber-todo
description: Track Timber worktree and repository TODO lists. Use when managing task lists for a worktree or its repository.
---

# Timber TODO

Each Timber worktree has its own `TODO.md` in its Git directory.
Each repository also has a shared `TODO.repo.md` in its common Git directory.
These notes do not pollute the checkout.
Repository TODO items do not affect worktree status counts.

## Locate the TODO file

Run from inside the worktree:

```bash
timber todo --path                # current worktree
timber todo feature/login --path  # another worktree in any repo
timber todo feature/login@timber --path
timber todo --repo --path                 # current worktree's repository
timber todo feature/login@timber --repo --path
```

This prints the absolute path of the selected TODO file, creating it when missing.
With `--repo`, the existing positional worktree selector identifies the repository; it is not a repository selector.
Do not guess the path; always resolve it with `--path` first.

## Edit items as Markdown checkboxes

Track items with Markdown checkbox list syntax:

```markdown
- [ ] Draft the migration plan
- [x] Add the --path flag
```

- `- [ ]` marks an open item.
- `- [x]` marks a completed item.
- Keep one item per line; add new items by appending lines.
- Never delete completed items; mark them `- [x]` instead.
