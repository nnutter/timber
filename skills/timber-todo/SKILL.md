---
name: timber-todo
description: Track per-worktree TODO lists stored in the worktree's git-dir TODO.md file. Use when managing task lists for the current worktree.
---

# Timber TODO

Each timber worktree has its own `TODO.md`, stored in the worktree's
git directory (inside the common git-dir) so notes never pollute the
checkout.

## Locate the TODO file

Run from inside the worktree:

```bash
timber todo --path                # current worktree
timber todo feature/login --path  # another worktree in any repo
timber todo feature/login@timber --path
```

This prints the absolute path of the worktree-specific `TODO.md`,
creating it when missing. Do not guess the path; always resolve it
with `--path` first.

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
