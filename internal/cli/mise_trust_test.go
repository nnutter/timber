package cli

import (
	"path/filepath"
	"testing"

	"github.com/nnutter/timber/internal/timber"
)

func TestCreateAutomaticallyTrustsOnlyEligibleMiseConfigs(t *testing.T) {
	parent := t
	var timberBin string
	for _, test := range []miseTrustCase{
		{name: "prompt yes", remote: "git@github.com:alice/project.git", miseFile: true, interactive: true, answer: "yes", wantPrompt: true, wantTrust: true},
		{name: "prompt without organizations", remote: "git@github.com:alice/project.git", miseFile: true, interactive: true, answer: "yes", wantPrompt: true, wantTrust: true, noOrganizations: true},
		{name: "prompt no", remote: "git@github.com:alice/project.git", miseFile: true, interactive: true, answer: "no", wantPrompt: true},
		{name: "prompt default no", remote: "git@github.com:alice/project.git", miseFile: true, interactive: true, wantPrompt: true},
		{name: "explicit refusal", remote: "git@github.com:alice/project.git", miseFile: true, interactive: true, explicitDisable: true},
		{name: "no prompt for other owner", remote: "git@github.com:stranger/project.git", miseFile: true, interactive: true},
		{name: "no prompt without mise", remote: "git@github.com:alice/project.git", interactive: true},
		{name: "refresh on creation", remote: "git@github.com:alice/project.git", enabled: true, miseFile: true, cache: "refresh", wantTrust: true},
		{name: "local branch tracking source", remote: "git@github.com:alice/project.git", enabled: true, miseFile: true, localStart: true, wantTrust: true},
		{name: "symlink config", remote: "git@github.com:alice/project.git", enabled: true, miseFile: true, symlink: true},
		{name: "user SSH", remote: "git@github.com:alice/project.git", enabled: true, miseFile: true, wantTrust: true},
		{name: "organization HTTPS", remote: "https://github.com/SECOND-ORG/project.git", enabled: true, miseFile: true, wantTrust: true},
		{name: "SSH URL", remote: "ssh://git@ssh.github.com:443/alice/project.git", enabled: true, miseFile: true, wantTrust: true},
		{name: "disabled", remote: "git@github.com:alice/project.git", miseFile: true},
		{name: "no mise file", remote: "git@github.com:alice/project.git", enabled: true},
		{name: "unrelated owner", remote: "git@github.com:stranger/project.git", enabled: true, miseFile: true},
		{name: "other host", remote: "git@gitlab.com:alice/project.git", enabled: true, miseFile: true, cache: "missing"},
		{name: "hostname suffix", remote: "https://github.com.evil.test/alice/project", enabled: true, miseFile: true},
		{name: "hostname prefix", remote: "https://evilgithub.com/alice/project", enabled: true, miseFile: true},
		{name: "file URL", remote: "file://github.com/alice/project", enabled: true, miseFile: true},
		{name: "local path", remote: "/github.com/alice/project", enabled: true, miseFile: true},
		{name: "unowned cache missing", remote: "git@github.com:stranger/project", enabled: true, miseFile: true, cache: "missing", wantWarn: "GitHub identity cache is unavailable"},
		{name: "missing cache", remote: "git@github.com:alice/project", enabled: true, miseFile: true, cache: "missing", wantWarn: "GitHub identity cache is unavailable"},
		{name: "corrupt cache", remote: "git@github.com:alice/project", enabled: true, miseFile: true, cache: "corrupt", wantWarn: "GitHub identity cache is unavailable"},
		{name: "expired cache", remote: "git@github.com:alice/project", enabled: true, miseFile: true, cache: "expired", wantWarn: "GitHub identity cache is unavailable"},
		{name: "mise failure", remote: "git@github.com:alice/project", enabled: true, miseFile: true, miseFails: true, wantTrust: true, wantWarn: "mise trust failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			runMiseTrustCase(t, parent, &timberBin, test)
		})
	}
}

// Exercise terminal detection and the persisted choice through the real CLI.
func interactiveCreate(t *testing.T, runtime timber.Runtime, bin, answer, branch string) commandResult {
	t.Helper()
	script := `import os, pty, select, signal, sys, time
pid, fd = pty.fork()
if pid == 0:
    os.chdir(sys.argv[2])
    os.execv(sys.argv[1], [sys.argv[1], "create", "--upstream", "source/main", sys.argv[4] + "@repo"])
output = b""
sent = False
try:
    deadline = time.monotonic() + 20
    while time.monotonic() < deadline:
        ready, _, _ = select.select([fd], [], [], 0.1)
        if not ready:
            continue
        try:
            chunk = os.read(fd, 8192)
        except OSError:
            break
        if not chunk:
            break
        output += chunk
        if b"[y/N] " in output and not sent:
            os.write(fd, (sys.argv[3] + "\n").encode())
            sent = True
    else:
        raise RuntimeError("timed out waiting for create: " + output.decode(errors="replace"))
    _, status = os.waitpid(pid, 0)
    sys.stdout.write(output.decode(errors="replace"))
    sys.exit(os.waitstatus_to_exitcode(status))
finally:
    os.close(fd)
    try:
        os.kill(pid, signal.SIGKILL)
    except ProcessLookupError:
        pass
`
	command := shellTestCommand(t, "python3", "-c", script, filepath.Join(bin, "timber"), runtime.HomeDirectory, answer, branch)
	command.Env = runtime.Environment
	output, err := command.CombinedOutput()
	return commandResult{stderr: string(output), err: err}
}
