---
name: timber-todo
description: Track Timber worktree and repository TODO lists. Use when managing task lists for a worktree or its repository.
---

# Timber TODO

Each Timber worktree has a `TODO.md` in its Git directory.
Each repository has a shared `TODO.md` in its common Git directory.
In a main checkout, the two scopes use the same file.
These notes do not pollute the checkout.
Repository TODO items do not affect status counts for linked worktrees.

## Locate the TODO file

Run from inside the worktree:

```bash
timber todo --path                # current worktree
timber todo feature/login --path  # another worktree in any repo
timber todo feature/login@timber --path
timber todo --repo --path                 # current worktree's repository
timber todo feature/login@timber --repo --path
```

This prints the absolute path of the selected TODO file.
A missing or empty file is initialized with a pull request stack template.
With `--repo`, the existing positional worktree selector identifies the repository; it is not a repository selector.
Do not guess the path; always resolve it with `--path` first.

## Edit items as Markdown checkboxes

List pull requests from bottom to top in the proposed stack.
Use top-level items for pull requests, nested items for their commits, and items nested under a commit for its notes or tasks.

```markdown
- [ ] Add a command to validate Git history
  - [ ] Add the check command
    - [x] Decide which trailers are required
```

- `- [ ]` marks an open item.
- `- [x]` marks a completed item.
- Keep one item per line; add new items by appending lines.
- Mark each TODO item `- [x]` when you complete it.
- Keep completed items in the list.
