package review

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type mirrorMiseTrustCase struct {
	name        string
	sourceTrust bool
	reviewTrust bool
	trustExit   string
	withoutMise bool
	wantArgs    string
	wantError   string
}

// fakeMise installs a mise stub that records trust state in a file, because
// mise stores trust per directory and the real state is user-global.
func fakeMise(t *testing.T) (logPath, trustedPath string) {
	t.Helper()
	dir := t.TempDir()
	logPath = filepath.Join(dir, "mise.log")
	trustedPath = filepath.Join(dir, "trusted")
	require.NoError(t, os.WriteFile(trustedPath, nil, 0o600))
	script := `#!/bin/sh
set -eu
printf '%s|%s\n' "$PWD" "$*" >> "$MISE_TEST_LOG"
if [ "$1" = "trust" ] && [ "$2" = "--show" ]; then
	if grep -qxF "$PWD" "$MISE_TEST_TRUSTED"; then
		printf '%s: trusted\n' "$PWD"
	else
		printf '%s: untrusted\n' "$PWD"
	fi
	exit 0
fi
if [ "$1" = "trust" ] && [ "$2" = "--untrust" ]; then
	if [ "${MISE_TEST_TRUST_EXIT:-0}" -ne 0 ]; then
		echo "mise untrust refused" >&2
		exit "$MISE_TEST_TRUST_EXIT"
	fi
	grep -vxF "$3" "$MISE_TEST_TRUSTED" > "$MISE_TEST_TRUSTED.tmp" || true
	mv "$MISE_TEST_TRUSTED.tmp" "$MISE_TEST_TRUSTED"
	exit 0
fi
if [ "$1" = "trust" ]; then
	if [ "${MISE_TEST_TRUST_EXIT:-0}" -ne 0 ]; then
		echo "mise trust refused" >&2
		exit "$MISE_TEST_TRUST_EXIT"
	fi
	printf '%s\n' "$2" >> "$MISE_TEST_TRUSTED"
	exit 0
fi
exit 64
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "mise"), []byte(script), 0o700))
	t.Setenv("MISE_TEST_LOG", logPath)
	t.Setenv("MISE_TEST_TRUSTED", trustedPath)
	t.Setenv("MISE_TEST_TRUST_EXIT", "0")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath, trustedPath
}

// resolvedTempDir returns a temporary directory with symlinks resolved. The
// fake mise reports the physical working directory that the shell resolves, and
// macOS makes the temporary directory reachable through a symlink.
func resolvedTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return dir
}

func writeMiseConfig(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "mise.toml"), []byte("[env]\nTIMBER_TEST = \"1\"\n"), 0o600))
}

func readMiseLog(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func TestMirrorMiseTrust(t *testing.T) {
	for _, test := range []mirrorMiseTrustCase{
		{name: "copies trusted source", sourceTrust: true, wantArgs: "trust"},
		{name: "leaves untrusted source alone"},
		{name: "skips already trusted review", sourceTrust: true, reviewTrust: true},
		{name: "untrusts for an untrusted source", reviewTrust: true, wantArgs: "trust --untrust"},
		{name: "skips already untrusted review"},
		{name: "reports trust failure", sourceTrust: true, trustExit: "1", wantArgs: "trust", wantError: "mirror mise trust in review checkout"},
		{name: "reports untrust failure", reviewTrust: true, trustExit: "1", wantArgs: "trust --untrust", wantError: "mirror mise trust in review checkout"},
		{name: "skips without mise", sourceTrust: true, withoutMise: true},
	} {
		t.Run(test.name, func(t *testing.T) { testMirrorMiseTrust(t, test) })
	}
}

func testMirrorMiseTrust(t *testing.T, test mirrorMiseTrustCase) {
	t.Helper()
	logPath, trustedPath := fakeMise(t)
	source, path := resolvedTempDir(t), resolvedTempDir(t)
	writeMiseConfig(t, source)
	writeMiseConfig(t, path)
	var trusted []string
	if test.sourceTrust {
		trusted = append(trusted, source)
	}
	if test.reviewTrust {
		trusted = append(trusted, path)
	}
	require.NoError(t, os.WriteFile(trustedPath, []byte(strings.Join(trusted, "\n")+"\n"), 0o600))
	t.Setenv("MISE_TEST_TRUST_EXIT", test.trustExit)
	if test.withoutMise {
		t.Setenv("PATH", t.TempDir())
	}
	repository := Repository{Dir: source, Environment: os.Environ()}
	assertMirrorMiseTrust(t, test, mirrorMiseTrust(t.Context(), repository, path), source, path, logPath)
}

func assertMirrorMiseTrust(t *testing.T, test mirrorMiseTrustCase, err error, source, path, logPath string) {
	t.Helper()
	if test.wantError != "" {
		require.ErrorContains(t, err, test.wantError)
	} else {
		require.NoError(t, err)
	}
	if test.withoutMise {
		require.NoFileExists(t, logPath)
		return
	}
	want := []string{source + "|trust --show", path + "|trust --show"}
	if test.wantArgs != "" {
		want = append(want, source+"|"+test.wantArgs+" "+path)
	}
	require.Equal(t, want, readMiseLog(t, logPath))
}

func TestMirrorMiseTrustSkipsReviewWithoutConfig(t *testing.T) {
	logPath, _ := fakeMise(t)
	source, path := resolvedTempDir(t), resolvedTempDir(t)
	writeMiseConfig(t, source)
	require.NoError(t, mirrorMiseTrust(t.Context(), Repository{Dir: source, Environment: os.Environ()}, path))
	require.NoFileExists(t, logPath)
}

func TestMiseListedAsTrustedReadsHomeRelativePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, "src", "project")
	require.True(t, miseListedAsTrusted("~/src/project: trusted\n", dir))
	require.False(t, miseListedAsTrusted("~/src/project: untrusted\n", dir))
	require.False(t, miseListedAsTrusted("/other/project: trusted\n", dir))
	require.False(t, miseListedAsTrusted("mise No trusted config files found.\n", dir))
}

func TestPrepareWorktreeMirrorsMiseTrust(t *testing.T) {
	_, trustedPath := fakeMise(t)
	r := testRepo(t)
	writeMiseConfig(t, r.Dir)
	gitTest(t, r.Dir, "add", "mise.toml")
	gitTest(t, r.Dir, "commit", "-m", "Add mise config")
	require.NoError(t, os.WriteFile(trustedPath, []byte(r.Dir+"\n"), 0o600))
	view, err := r.Snapshot(t.Context())
	require.NoError(t, err)
	path, err := WorktreePath(r.Dir)
	require.NoError(t, err)
	require.NoError(t, prepareWorktree(t.Context(), r, path, view.Stacks[0]))
	data, err := os.ReadFile(trustedPath)
	require.NoError(t, err)
	require.Contains(t, strings.Fields(string(data)), path)
	require.Equal(t, view.Stacks[0].Tip, gitTest(t, path, "rev-parse", "HEAD"))
}
