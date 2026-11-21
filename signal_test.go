package main

import (
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nnutter/timber/internal/testenv"
)

func TestMain(m *testing.M) {
	if filepath.Base(os.Args[0]) == "git" {
		if ready, ok := os.LookupEnv("TIMBER_TEST_GIT_READY"); ok {
			runFakeGit(ready)
			os.Exit(1)
		}
	}
	os.Exit(m.Run())
}

func runFakeGit(ready string) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGQUIT, syscall.SIGTERM)
	defer signal.Stop(signals)
	if err := os.WriteFile(ready, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		panic(err)
	}
	select {
	case sig := <-signals:
		if err := os.WriteFile(os.Getenv("TIMBER_TEST_GIT_SIGNAL"), []byte(sig.String()), 0o600); err != nil {
			panic(err)
		}
		if os.Getenv("TIMBER_TEST_GIT_HANG") == "true" {
			<-time.After(10 * time.Second)
		}
	case <-time.After(10 * time.Second):
	}
}

func TestTimberForwardsSignalsToGit(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "timber")
	build := exec.CommandContext(t.Context(), "go", testenv.BuildArgs(binary, ".")...)
	output, err := build.CombinedOutput()
	require.NoError(t, err, "%s", output)
	testBinary, err := os.Executable()
	require.NoError(t, err)
	require.NoError(t, os.Symlink(testBinary, filepath.Join(dir, "git")))

	for _, tc := range []struct {
		name string
		sig  os.Signal
		hang bool
	}{
		{name: "interrupt", sig: os.Interrupt},
		{name: "quit", sig: syscall.SIGQUIT},
		{name: "terminate", sig: syscall.SIGTERM},
		{name: "unresponsive Git", sig: syscall.SIGTERM, hang: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertGitShutdown(t, binary, tc.sig, tc.hang)
		})
	}
}

func assertGitShutdown(t *testing.T, binary string, sig os.Signal, hang bool) {
	t.Helper()
	home := t.TempDir()
	ready := filepath.Join(home, "ready")
	received := filepath.Join(home, "signal")
	cmd := exec.CommandContext(t.Context(), binary, "repo", "import", home)
	cmd.Env = append(os.Environ(),
		"PATH="+filepath.Dir(binary)+string(os.PathListSeparator)+os.Getenv("PATH"),
		"HOME="+home, "XDG_CONFIG_HOME="+home, "XDG_DATA_HOME="+home,
		"TIMBER_TEST_GIT_READY="+ready, "TIMBER_TEST_GIT_SIGNAL="+received,
		"TIMBER_TEST_GIT_HANG="+strconv.FormatBool(hang),
	)
	output, err := os.Create(filepath.Join(home, "timber.log"))
	require.NoError(t, err)
	defer func() { _ = output.Close() }()
	cmd.Stdout, cmd.Stderr = output, output
	require.NoError(t, cmd.Start())
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	defer func() { _ = cmd.Process.Kill() }()

	var pid int
	require.Eventually(t, func() bool {
		data, err := os.ReadFile(ready)
		if err != nil {
			return false
		}
		pid, err = strconv.Atoi(string(data))
		return err == nil
	}, 5*time.Second, 10*time.Millisecond, "Git did not become ready")
	git, err := os.FindProcess(pid)
	require.NoError(t, err)
	defer func() { _ = git.Kill(); _ = git.Release() }()

	require.NoError(t, cmd.Process.Signal(sig))
	select {
	case err := <-done:
		require.Error(t, err, "timber reported success after interruption")
	case <-time.After(5 * time.Second):
		require.FailNow(t, "timber did not exit after interruption")
	}
	data, err := os.ReadFile(received)
	require.NoError(t, err, "Git did not receive %s", sig)
	assert.Equal(t, sig.String(), string(data))
	assert.Error(t, git.Signal(syscall.Signal(0)), "Git was still running after timber exited")
}
