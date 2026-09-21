package timber

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// generatedZshCommand runs the generated wrapper in an isolated shell. Only
// the external timber executable is replaced; the wrapper itself is real.
func generatedZshCommand(t *testing.T, fakeTimber string, program string, args ...string) *exec.Cmd {
	t.Helper()
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh is not installed")
	}
	outDir := resolvedTempDir(t)
	require.NoError(t, runTimberCommand(t, "generate", "zsh", "--out", outDir).err)
	binDir := resolvedTempDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "timber"), []byte(fakeTimber), 0o755))

	arguments := append([]string{"-f", "-c", program, "--", filepath.Join(outDir, "t")}, args...)
	command := testCommand(t, zsh, arguments...)
	command.Env = replaceTestEnvironment(command.Env, "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return command
}
