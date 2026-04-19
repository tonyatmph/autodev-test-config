package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDoesNotRequireRuntimeImageCatalogInConfig(t *testing.T) {
	root := t.TempDir()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	installFixedConfigForTest(t, filepath.Join(repoRoot, "TEST"))
	cfg := `{
  "paths": {
    "root_dir": ".",
    "data_dir": "./data",
    "work_order_repo": "./work-orders"
  },
  "gitlab": { "token_name": "token" },
  "stores": { "ratchet_postgres_dsn": "noop" },
  "secrets": { "local_keychain_service": "autodev" }
}`
	configPath := filepath.Join(root, "autodev.json")
	writeFile(t, configPath, cfg)

	if _, err := Load(configPath); err != nil {
		t.Fatalf("expected config load to succeed without runtime image catalog in config, got %v", err)
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func installFixedConfigForTest(t *testing.T, sourceRoot string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	targetRoot := filepath.Join(home, ".autodev", "config")
	if err := copyTree(sourceRoot, targetRoot); err != nil {
		t.Fatalf("install fixed config: %v", err)
	}
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}
