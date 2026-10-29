package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func installFakeHerdrSpace(t *testing.T, logPath string) string {
	t.Helper()

	binDir := resolvedTempDir(t)
	scriptPath := filepath.Join(binDir, "herdr")
	script := fmt.Sprintf(`#!/bin/sh
first=1
for arg in "$@"; do
  if [ "$first" -eq 1 ]; then
    first=0
  else
    printf '\037' >> %q
  fi
  printf '%%s' "$arg" >> %q
done
printf '\n' >> %q

operation="$1 $2"
tab_label=""
workspace_id=""
list_cwd=""
open_path=""
previous=""
for arg in "$@"; do
  if [ "$previous" = "--label" ]; then
    tab_label="$arg"
  elif [ "$previous" = "--workspace" ]; then
    workspace_id="$arg"
  elif [ "$previous" = "--cwd" ]; then
    list_cwd="$arg"
  elif [ "$previous" = "--path" ]; then
    open_path="$arg"
  fi
  previous="$arg"
done
if [ "$operation" = "${FAKE_HERDR_FAIL:-}" ] ||
   { [ "$operation" = "tab create" ] && [ "$tab_label" = "${FAKE_HERDR_FAIL_TAB_LABEL:-}" ]; }; then
  echo "fake herdr failure" >&2
  exit 1
fi
if [ "$operation" = "${FAKE_HERDR_MALFORM:-}" ]; then
  printf '{'
  exit 0
fi

case "$operation" in
  "workspace create")
    printf '{"result":{"workspace":{"workspace_id":"w1"},"tab":{"tab_id":"w1:t1"},"root_pane":{"pane_id":"w1:p1"}}}'
    ;;
  "workspace get")
    printf '{"result":{"workspace":{"workspace_id":"%%s","label":"%%s"}}}' "$3" "${FAKE_HERDR_PARENT_LABEL:-repo}"
    ;;
  "worktree list")
    if [ -z "${FAKE_HERDR_PARENT_ID:-}" ]; then
      parent_ref="null"
    else
      parent_ref="\"$FAKE_HERDR_PARENT_ID\""
    fi
    printf '{"result":{"source":{"repo_key":"test","repo_name":"repo","repo_root":"%%s","source_checkout_path":"%%s","source_workspace_id":%%s},"worktrees":[]}}' "$list_cwd" "$list_cwd" "$parent_ref"
    ;;
  "worktree open")
    if [ "${FAKE_HERDR_ALREADY_OPEN:-}" = "1" ]; then
      already="true"
    else
      already="false"
    fi
    printf '{"result":{"workspace":{"workspace_id":"w2"},"tab":{"tab_id":"w2:t1"},"root_pane":{"pane_id":"w2:p1"},"worktree":{"path":"%%s"},"already_open":%%s}}' "$open_path" "$already"
    ;;
  "pane current")
    printf '{"result":{"pane":{"workspace_id":"w9","tab_id":"w9:t1","pane_id":"w9:p1"}}}'
    ;;
  "tab create")
    if [ "$tab_label" = "Shell" ]; then
      printf '{"result":{"tab":{"tab_id":"%%s:t3"},"root_pane":{"pane_id":"%%s:p3"}}}' "$workspace_id" "$workspace_id"
    else
      printf '{"result":{"tab":{"tab_id":"%%s:t2"},"root_pane":{"pane_id":"%%s:p2"}}}' "$workspace_id" "$workspace_id"
    fi
    ;;
  *)
    printf '{"result":{}}'
    ;;
esac
`, logPath, logPath, logPath)
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))

	return scriptPath
}

func installFakeGh(t *testing.T) string {
	t.Helper()

	binDir := resolvedTempDir(t)
	scriptPath := filepath.Join(binDir, "gh")
	script := `#!/bin/sh
operation="$1 $2"
if [ "$operation" = "auth status" ]; then
  if [ "${FAKE_GH_AUTH_OK:-1}" = "1" ]; then
    exit 0
  fi
  echo "not logged in" >&2
  exit 1
fi
if [ "$operation" = "pr list" ]; then
  printf '%s' "${FAKE_GH_PR_JSON:-[]}"
  exit 0
fi
echo "unexpected gh call: $*" >&2
exit 1
`
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))

	return scriptPath
}

func fakeHerdrLogLine(args ...string) string {
	return strings.Join(args, "\x1f")
}

func readFakeHerdrLog(t *testing.T, logPath string) []string {
	t.Helper()
	contents, err := os.ReadFile(logPath)
	require.NoError(t, err)
	return strings.Split(strings.TrimSpace(string(contents)), "\n")
}

func parentDashboardLogLine() string {
	return fakeHerdrLogLine("pane", "run", "w1:p1", "while :; do clear; timber list '@"+testRepoName+"' --pr; sleep 60; done")
}
