---
name: timber
description: Manage Git worktrees with the Timber CLI.
---

# Timber

Use Timber to register bare repositories and create, inspect, and remove
branch-attached Git worktrees. This is `github.com/nnutter/timber`, not a
logging service. For detailed checklist editing, use the separate
`timber-todo` skill.

## When to use

- Create an isolated checkout for a task or locate an existing worktree.
- Inspect worktree status and TODO progress across registered repositories.
- Clean up a specific finished worktree after confirming what will be removed.
- Do not use for detached worktrees or unrelated Git hosting operations.

## Prerequisites and configuration

- `timber` and Git must be on `PATH`. Install Timber with
  `go install github.com/nnutter/timber@latest` (requires a compatible Go
  toolchain; see the repository's `go.mod`). Verify with `timber --help`.
- Remote registration and worktree creation need access to the origin;
  creation fetches origin even when reusing an existing local branch.
  Use the user's existing Git authentication. Never embed credentials.
- `gh` authenticated to GitHub is optional for `list --pr` and automatic
  mise trust / hk eligibility checks. Herdr is optional; use `--no-herdr`
  for headless creation.
- `timber config` shows settings and creates the configuration file if absent:
  `$XDG_CONFIG_HOME/timber/config.toml`, falling back to
  `~/.config/timber/config.toml`.
- Bare repositories live at `$TIMBER_WORKTREE_ROOT/<repo-name>/.git`
  (fallback `~/worktrees/<repo-name>/.git`). `TIMBER_WORKTREE_ROOT`
  overrides the default `~/worktrees` checkout root. Preserve the user's
  environment.
- Automatic mise trust and hk installation default to disabled. Do not enable
  them without approval: they permit repository-defined code execution.
  Already-enabled settings still apply to headless creation; `--no-herdr`
  does not disable them. Non-interactive creation does not ask setup questions.

## How to run

Use the terminal tool (or the host agent's equivalent command runner), invoking
`timber` directly rather than relying on the interactive zsh `t` function.
For example: `terminal(command="timber list --json")`.
All commands below are terminal commands. Replace example selectors with
names returned by `timber repo list` and `timber list --json`.

```bash
# Discover registrations and checkouts without a picker.
timber repo list
timber list --json
timber list @owner/repo --json

# Only if registration is needed and authorized:
timber repo add owner/repo

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

# Preview cleanup before proposing any deletion.
timber prune @owner/repo --dry-run
# Only after approval for this exact worktree and its branch:
timber remove feature/login@owner/repo
```

## Procedure

1. Discover the registered repository and existing worktrees. Use the full
   registered name (possibly `owner/repo`), not a guessed short alias.
   A selector is `<worktree>@<repo>`; `@<repo>` pins repository-only operations.
   Explicit qualification avoids interactive pickers and name ambiguity.
2. Reuse an appropriate existing checkout or create the approved branch with
   `--no-herdr`. The worktree name equals the branch for Timber-created
   worktrees and cannot contain `@`. Inspect stderr warnings as well as the
   exit code; setup failures can leave a successfully created checkout.
3. Read the path printed by `create`, or the `path` in `list --json`.
   Do not reconstruct it: custom paths and manually linked worktrees exist.
   Set the terminal working directory explicitly for subsequent calls;
   the CLI cannot change the parent agent's working directory.
4. Work in that checkout. Verify with `git status --short --branch` and
   `timber list @owner/repo --json`. Treat `statusError: true` as unknown
   status, not permission to delete. JSON also includes `clean`, `merged`,
   `upstream`, `ahead`, `behind`, `todoDone`, and `todoTotal`.
5. For cleanup, inspect Git status, preserve needed commits/files/notes, and
   show the scoped `prune --dry-run` result before requesting approval.
   `prune` removes clean, merged worktrees; `remove` also deletes the branch.
   Never infer user approval from `merged`, and never retry with `--force`
   merely because normal removal refused. Move the runner outside a checkout
   before deleting it. Re-list afterward and verify only approved targets vanished.

## Pitfalls and limitations

- `list` and `prune` default to every registered repository: always qualify
  cleanup with `@<repo>`. `repo remove`, `repo rename`, `repo import`, and
  `repo fix` are broader operations requiring explicit approval. `repo fix`
  moves bare repositories into the worktree root and rewrites worktree
  links. Import relocates existing clean checkouts and requires a `trash`
  CLI. Do not use import as simple registration.
- Detached worktrees are not managed. Names for manually linked checkouts can
  change when collisions appear; rediscover before acting.
- The zsh wrapper `t` supplies shell integration and `cd` behavior. Do not
  assume `timber switch` changes the agent runner's working directory, and
  avoid `tui` or interactive pickers in unattended sessions.
- `todo --path` and `todo --json` create a missing TODO.md inside the Git
  directory. Resolve the path instead of writing a checkout-root TODO.md.
  Notes are local worktree state, not committed source; preserve them before removal.
- A failed create can leave a worktree behind (for example after optional
  setup fails). Re-list before retrying; do not force-remove it automatically.

## Verification

Confirm the expected repository, name, and real checkout path in `list --json`;
check the branch with Git inside that checkout. After approved removal,
confirm that selector is absent and unrelated worktrees remain. Report the
path/branch and any warnings, not merely the command's exit status.
