package secrets

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type stubProvider struct {
	value Value
	err   error
}

func (s stubProvider) Resolve(context.Context, string) (Value, error) {
	return s.value, s.err
}

func TestNormalizeKeychainName(t *testing.T) {
	if got := NormalizeKeychainName("gitops/write-token"); got != "gitops-write-token" {
		t.Fatalf("unexpected keychain name: %s", got)
	}
}

func TestChainFallsBackToSecondProvider(t *testing.T) {
	chain := NewChain(
		stubProvider{err: ErrNotFound},
		stubProvider{value: Value{Name: "deploy-token", Source: "gcp", Value: "secret"}},
	)
	value, err := chain.Resolve(context.Background(), "deploy-token")
	if err != nil {
		t.Fatal(err)
	}
	if value.Source != "gcp" {
		t.Fatalf("unexpected source: %s", value.Source)
	}
}

func TestChainReturnsNotFound(t *testing.T) {
	chain := NewChain(stubProvider{err: ErrNotFound})
	if _, err := chain.Resolve(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestFileProviderResolvesFixtureValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "smoke-secrets.json")
	if err := os.WriteFile(path, []byte("{\"gitlab-write-token\":\"fixture-token\"}"), 0o644); err != nil {
		t.Fatal(err)
	}

	provider, err := NewFileProvider(path)
	if err != nil {
		t.Fatal(err)
	}
	value, err := provider.Resolve(context.Background(), "gitlab-write-token")
	if err != nil {
		t.Fatal(err)
	}
	if value.Value != "fixture-token" {
		t.Fatalf("unexpected secret value: %q", value.Value)
	}
}
