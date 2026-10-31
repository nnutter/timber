---
name: timber
description: Manage Git worktrees with the Timber CLI.
---

# Timber

Use Timber to register bare repositories and create, inspect, and remove branch-attached Git worktrees.
This is `github.com/nnutter/timber`, not a logging service.
For detailed checklist editing, use the separate `timber-todo` skill.

## When to use

- Create an isolated checkout for a task or locate an existing worktree.
- Inspect worktree status and TODO progress across registered repositories.
- Clean up a specific finished worktree after confirming what will be removed.
- Open a pull request for a worktree branch when the user asks for one.
- Do not use for detached worktrees, or for Git hosting work beyond creating a pull request (review, merge, or release).

## Prerequisites and configuration

- `timber` and Git must be on `PATH`.
  Install Timber with `go install github.com/nnutter/timber@latest` (requires a compatible Go toolchain; see the repository's `go.mod`).
  Verify with `timber --help`.
- Remote registration and worktree creation need access to the origin; creation fetches origin even when reusing an existing local branch.
  Use the user's existing Git authentication.
  Never embed credentials.
- `gh` authenticated to GitHub is optional for `list --pr` and automatic mise trust / hk eligibility checks.
  Herdr is optional; use `--no-herdr` for headless creation.
- `timber pr create` needs a GitHub token for `github.com`.
  It reads `GH_TOKEN`, then `GITHUB_TOKEN`, then the credential that `gh auth login` stored in the system keyring.
  Timber never reads a token from a configuration file, and never accepts one on a command line.
  Do not write a secret into a Timber or gh configuration file.
  On a machine without a keyring, such as a headless server or CI, set `GH_TOKEN`.
- `timber config` shows settings and creates the configuration file if absent:
  `$XDG_CONFIG_HOME/timber/config.toml`, falling back to
  `~/.config/timber/config.toml`.
- Bare repositories live at `$TIMBER_WORKTREE_ROOT/<repo-name>/.git` (fallback `~/worktrees/<repo-name>/.git`). `TIMBER_WORKTREE_ROOT` overrides the default `~/worktrees` checkout root.
  Preserve the user's environment.
- Automatic mise trust and hk installation default to disabled.
  Do not enable them without approval: they permit repository-defined code execution.
  Already-enabled settings still apply to headless creation; `--no-herdr` does not disable them.
  Non-interactive creation does not ask setup questions.

## How to run

Use the terminal tool (or the host agent's equivalent command runner), invoking `timber` directly rather than relying on the interactive zsh `t` function.
For example: `terminal(command="timber list --json")`.
All commands below are terminal commands.
Replace example selectors with names returned by `timber repo list` and `timber list --json`.

```bash
# Discover registrations and checkouts without a picker.
timber repo list
timber list --json
timber list @owner/repo --json

# Only if registration is needed and authorized:
timber repo add owner/repo
# Create a local repository without a remote, with one empty commit:
timber repo init demo -b main

# Create an explicitly named worktree, with no Herdr window.
timber create feature/login@owner/repo --no-herdr
# Or choose an exact checkout path (quote paths containing spaces):
timber create feature/custom@owner/repo ../checkout --no-herdr
# Override the base/upstream when needed:
timber create feature/fix@owner/repo --upstream origin/main --no-herdr

# Inspect; --pr additionally requires gh and GitHub access.
timber list @owner/repo --json
timber list @owner/repo --pr --json

# Resolve per-worktree notes without opening an editor.
timber todo feature/login@owner/repo --path
timber todo feature/login@owner/repo --json
# Resolve the repository-wide list using a worktree selector.
timber todo feature/login@owner/repo --repo --path
timber todo --repo --json

# Open a GitHub pull request for a pushed branch (title is required).
timber pr create feature/login@owner/repo --title "Add login" --body "Adds login."
# Merge somewhere other than the branch's upstream, or open a draft:
timber pr create feature/login@owner/repo --title "Add login" --base develop --draft

# Preview cleanup before proposing any deletion.
timber prune @owner/repo --dry-run
# Only after approval for this exact worktree and its branch:
timber remove feature/login@owner/repo
```

## Procedure

1. Discover the registered repository and existing worktrees.
   Use the full registered name (possibly `owner/repo`), not a guessed short alias.
   A selector is `<worktree>@<repo>`; `@<repo>` pins repository-only operations.
   Explicit qualification avoids interactive pickers and name ambiguity.
1. Reuse an appropriate existing checkout or create the approved branch with
   `--no-herdr`.
   The worktree name equals the branch for Timber-created worktrees and cannot contain `@`.
   Inspect stderr warnings as well as the exit code; setup failures can leave a successfully created checkout.
1. Read the path printed by `create`, or the `path` in `list --json`.
   Do not reconstruct it: custom paths and manually linked worktrees exist.
   Set the terminal working directory explicitly for subsequent calls; the CLI cannot change the parent agent's working directory.
1. Work in that checkout.
   Verify with `git status --short --branch` and `timber list @owner/repo --json`.
   Treat `statusError: true` as unknown status, not permission to delete.
   JSON also includes `clean`, `merged`, `upstream`, `ahead`, `behind`, `todoDone`, and `todoTotal`.
1. For a pull request, only when the user asks for one.
   Verify the branch and its commits first, then run `timber pr create` with an explicit selector, a `--title` that follows the repository's commit subject style, and a `--body` that explains why.
   The command never pushes: the branch must already be at the worktree's commit on GitHub, and only `github.com` remotes are supported.
   Report the printed URL.
1. For cleanup, inspect Git status, preserve needed commits/files/notes, and show the scoped `prune --dry-run` result before requesting approval.
   `prune` removes clean, merged worktrees; `remove` also deletes the branch.
   Never infer user approval from `merged`, and never retry with `--force` merely because normal removal refused.
   Move the runner outside a checkout before deleting it.
   Re-list afterward and verify only approved targets vanished.

## Pitfalls and limitations

- `pr create` creates a pull request only; it does not push, change the worktree, or open a browser.
  Push the branch first with the user's existing Git authentication.
  A branch that GitHub does not have, or has at a different commit, fails with the `git push` command to run.
  Forks and GitHub Enterprise hosts are not supported: the pull request head and base must be branches of the same `github.com` repository.
- `list` and `prune` default to every registered repository: always qualify cleanup with `@<repo>`. `repo remove`, `repo rename`, `repo import`, and `repo fix` are broader operations requiring explicit approval. `repo fix` moves bare repositories into the worktree root and rewrites worktree links.
  Import relocates existing clean checkouts and requires a `trash`
  CLI.
  Do not use import as simple registration.
- A repository created with `repo init` has no `origin`, so its worktree branches have no upstream and `remove` refuses them without `--force`.
  Add a remote, then fetch, before relying on upstream or pull request status.
- Detached worktrees are not managed.
  Names for manually linked checkouts can change when collisions appear; rediscover before acting.
- The zsh wrapper `t` supplies shell integration and `cd` behavior.
  Do not assume `timber switch` changes the agent runner's working directory, and avoid `tui` or interactive pickers in unattended sessions.
- `todo --path` and `todo --json` create a missing TODO file inside the Git directory.
  Without `--repo`, the file is the worktree's `TODO.md`; with `--repo`, it is `TODO.repo.md` in the common Git directory.
  The positional selector still selects a worktree and thereby identifies its repository.
  Repository TODO items do not affect worktree status counts.
  Resolve the path instead of writing a checkout-root TODO.md.
  Notes are local Git state, not committed source; preserve worktree notes before removal.
- A failed create can leave a worktree behind (for example after optional setup fails).
  Re-list before retrying; do not force-remove it automatically.

## Verification

Confirm the expected repository, name, and real checkout path in `list --json`; check the branch with Git inside that checkout.
After `pr create`, confirm the printed URL is a pull request for the intended repository, branch, and base.
After approved removal, confirm that selector is absent and unrelated worktrees remain.
Report the path/branch and any warnings, not merely the command's exit status.
