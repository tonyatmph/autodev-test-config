package runner

import (
	"context"

	repoplane "g7.mph.tech/mph-tech/autodev/internal/repos"
)

func repoCandidates(rootDir, projectPath string, extraRoots ...string) []string {
	return repoplane.RepoCandidates(rootDir, projectPath, extraRoots...)
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	return repoplane.GitOutput(ctx, dir, args...)
}
