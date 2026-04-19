package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return root
}

func requirePlatformRuntimeArtifact(t *testing.T, root string) string {
	t.Helper()
	path := filepath.Join(root, "bin", "autodev-stage-runtime")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("platform runtime artifact missing at %s; run `make build-runtime` first: %v", path, err)
	}
	return path
}
