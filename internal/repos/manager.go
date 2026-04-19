package repos

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"g7.mph.tech/mph-tech/autodev/internal/model"
)

type Manager struct {
	rootDir   string
	dataDir   string
	repoRoots []string
}

func NewManager(rootDir, dataDir string, repoRoots []string) *Manager {
	return &Manager{
		rootDir:   rootDir,
		dataDir:   dataDir,
		repoRoots: append([]string(nil), repoRoots...),
	}
}

func (m *Manager) RunRepoPath(runID string, repo model.RepoTarget) string {
	if strings.TrimSpace(repo.ProjectPath) == "" || strings.TrimSpace(repo.MaterializationPath) == "" {
		return ""
	}
	path := strings.ReplaceAll(repo.MaterializationPath, "{run_id}", runID)
	return filepath.Clean(path)
}

func (m *Manager) ResolveSourcePath(projectPath string) string {
	for _, candidate := range RepoCandidates(m.rootDir, projectPath, m.repoRoots...) {
		if info, err := os.Stat(filepath.Join(candidate, ".git")); err == nil && info.IsDir() {
			return candidate
		}
	}
	return ""
}

func (m *Manager) EnsureRunRepo(ctx context.Context, runID string, repo model.RepoTarget) (string, string, error) {
	if strings.TrimSpace(repo.ProjectPath) == "" {
		return "", "", fmt.Errorf("repo target is missing project_path")
	}
	if strings.TrimSpace(repo.MaterializationPath) == "" {
		return "", "", fmt.Errorf("repo %s is missing materialization_path", repo.ProjectPath)
	}
	if strings.TrimSpace(repo.Ref) == "" {
		return "", "", fmt.Errorf("repo %s is missing ref", repo.ProjectPath)
	}
	dest := m.RunRepoPath(runID, repo)
	if dest == "" {
		return "", "", fmt.Errorf("repo %s did not resolve a materialization path", repo.ProjectPath)
	}
	if _, ok := sourceStampFromPath(ctx, dest, repo); ok {
		return dest, "resolved", nil
	}
	source := m.ResolveSourcePath(repo.ProjectPath)
	if source == "" {
		return "", "", fmt.Errorf("repo %s could not be resolved from configured repo roots", repo.ProjectPath)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", "", fmt.Errorf("create run repo parent: %w", err)
	}
	if _, err := os.Stat(dest); err == nil {
		if err := os.RemoveAll(dest); err != nil {
			return "", "", fmt.Errorf("reset run repo dir: %w", err)
		}
	}
	if _, err := GitOutput(ctx, filepath.Dir(dest), "clone", "--quiet", "--no-hardlinks", source, dest); err != nil {
		return "", "", fmt.Errorf("clone %s into run repo: %w", repo.ProjectPath, err)
	}
	if err := configureCommitIdentity(ctx, dest); err != nil {
		return "", "", err
	}
	if err := checkoutResolvedRef(ctx, dest, repo); err != nil {
		return "", "", err
	}
	return dest, "resolved", nil
}

func configureCommitIdentity(ctx context.Context, dir string) error {
	if _, err := GitOutput(ctx, dir, "config", "user.name", "autodev"); err != nil {
		return fmt.Errorf("set git user.name: %w", err)
	}
	if _, err := GitOutput(ctx, dir, "config", "user.email", "autodev@example.invalid"); err != nil {
		return fmt.Errorf("set git user.email: %w", err)
	}
	return nil
}

func checkoutResolvedRef(ctx context.Context, dir string, repo model.RepoTarget) error {
	ref := strings.TrimSpace(repo.Ref)
	if _, err := GitOutput(ctx, dir, "checkout", "--quiet", ref); err != nil {
		return fmt.Errorf("checkout %s for %s: %w", ref, repo.ProjectPath, err)
	}
	return nil
}
