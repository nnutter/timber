package timber

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	coreconfig "github.com/nnutter/timber/internal/timber/config"
)

type config = coreconfig.Settings

func writeConfig(path string, settings config) error {
	var contents bytes.Buffer
	if err := toml.NewEncoder(&contents).Encode(settings); err != nil {
		return err
	}
	return writePrivateFile(path, contents.Bytes())
}

// Replace complete files so concurrent readers never see a partial document.
func writePrivateFile(path string, contents []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".timber-*")
	if err != nil {
		return err
	}
	// Cleanup is best effort; the temporary file is already gone after rename.
	defer func() { _ = os.Remove(file.Name()) }()
	_, writeErr := file.Write(contents)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
