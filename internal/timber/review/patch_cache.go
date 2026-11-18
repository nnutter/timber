package review

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// The cache is disposable and independent of review state. Each entry includes
// the fingerprint version, immutable endpoints, and Git's effective diff inputs.
type patchCache struct {
	dir, context    string
	attributesByTip map[string]string
	writable        bool
}

func (c *patchCache) hash(ctx context.Context, r Repository, base, tip string) (string, error) {
	if c == nil {
		return r.patchHash(ctx, base, tip)
	}
	key := fmt.Sprintf("%x.patch", sha256.Sum256([]byte(c.context+"\x00"+c.attributesByTip[tip]+"\x00"+base+"\x00"+tip)))
	path := filepath.Join(c.dir, key)
	if hash, ok := readPatchCache(path); ok {
		r.logger().Debug("Patch cache hit", "tip", tip)
		return hash, nil
	}
	r.logger().Debug("Patch cache miss", "tip", tip)
	hash, err := r.patchHash(ctx, base, tip)
	if err != nil {
		return "", err
	}
	if c.writable {
		if err := writePatchCache(path, hash); err != nil {
			// Do not report the same unavailable cache once per stack.
			c.writable = false
			r.logger().Debug("Patch cache write failed", "error", err)
		}
	}
	return hash, nil
}

func patchCachePath(view Snapshot) string {
	return filepath.Join(filepath.Dir(filepath.Dir(view.Path)), "cache", "patch-v2")
}

func newPatchCache(ctx context.Context, r Repository, view Snapshot, repo string) *patchCache {
	if len(view.Stacks) == 0 {
		return nil
	}
	root := r
	root.Dir = repo
	config, err := root.gitOutput(ctx, "config", "--null", "--list")
	if err != nil {
		r.logger().Debug("Patch cache unavailable", "error", err)
		return nil
	}
	version, err := root.git(ctx, "--version")
	if err != nil {
		r.logger().Debug("Patch cache unavailable", "error", err)
		return nil
	}
	inputs := []string{"patch-context-v3", version, config, environmentValue(r.Environment, "GIT_DIFF_OPTS"), environmentValue(r.Environment, "GIT_ATTR_SOURCE")}
	attributes, err := stackPatchAttributes(ctx, root, view.Stacks)
	if err != nil {
		r.logger().Debug("Patch cache unavailable", "error", err)
		return nil
	}
	cache := new(patchCache{
		dir:             patchCachePath(view),
		context:         fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(inputs, "\x00")))),
		attributesByTip: attributes,
	})
	if err := os.MkdirAll(cache.dir, 0o700); err != nil {
		r.logger().Debug("Patch cache is read-only", "error", err)
	} else {
		cache.writable = true
	}
	return cache
}

func readPatchCache(path string) (string, bool) {
	file, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, 256))
	if err != nil {
		return "", false
	}
	value := strings.TrimSuffix(string(data), "\n")
	if value == "empty" {
		return "", true
	}
	if strings.HasPrefix(value, "v2:") && patchHashPattern.MatchString(value) {
		return value, true
	}
	return "", false
}

func writePatchCache(path, hash string) (err error) {
	file, err := os.CreateTemp(filepath.Dir(path), ".patch-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(file.Name()) }()
	if hash == "" {
		hash = "empty"
	}
	_, writeErr := io.WriteString(file, hash+"\n")
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(file.Name(), path)
}
