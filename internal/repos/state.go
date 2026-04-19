package repos

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"strings"

	"g7.mph.tech/mph-tech/autodev/internal/model"
)

func RepoCandidates(rootDir, projectPath string, extraRoots ...string) []string {
	if strings.TrimSpace(projectPath) == "" {
		return nil
	}
	if filepath.IsAbs(projectPath) {
		return []string{projectPath}
	}
	basename := filepath.Base(projectPath)
	seen := make(map[string]struct{})
	var candidates []string
	add := func(root string) {
		if root == "" {
			return
		}
		for _, candidate := range []string{
			filepath.Join(root, projectPath),
			filepath.Join(root, basename),
		} {
			candidate = filepath.Clean(candidate)
			if _, ok := seen[candidate]; ok {
				continue
			}
			seen[candidate] = struct{}{}
			candidates = append(candidates, candidate)
		}
	}
	for _, root := range repoRoots(rootDir, extraRoots) {
		add(root)
	}
	return candidates
}

func GitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = gitTransportEnv()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return stdout.String(), nil
}

func gitTransportEnv() []string {
	return []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME=/nonexistent",
		"LANG=C",
		"LC_ALL=C",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_ASKPASS=/bin/false",
		"SSH_ASKPASS=/bin/false",
		"GCM_INTERACTIVE=Never",
		"GIT_SSH_COMMAND=ssh -F /dev/null -o BatchMode=yes -o StrictHostKeyChecking=yes",
	}
}

func sourceStampFromPath(ctx context.Context, path string, repo model.RepoTarget) (model.SourceStamp, bool) {
	commitSHA, err := GitOutput(ctx, path, "rev-parse", "HEAD")
	if err != nil {
		return model.SourceStamp{}, false
	}
	status, err := GitOutput(ctx, path, "status", "--porcelain")
	if err != nil {
		return model.SourceStamp{}, false
	}
	treeState := "clean"
	if strings.TrimSpace(status) != "" {
		treeState = "dirty"
	}
	return model.SourceStamp{
		ProjectPath:   repo.ProjectPath,
		DefaultBranch: repo.DefaultBranch,
		CommitSHA:     strings.TrimSpace(commitSHA),
		TreeState:     treeState,
	}, true
}

func repoRoots(rootDir string, extraRoots []string) []string {
	roots := []string{rootDir, filepath.Dir(rootDir)}
	for _, entry := range extraRoots {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		roots = append([]string{entry}, roots...)
	}
	return dedupeStrings(roots)
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = filepath.Clean(value)
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
