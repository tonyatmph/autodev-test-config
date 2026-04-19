package locks

import (
	"context"
	"fmt"
	"time"

	"g7.mph.tech/mph-tech/autodev/internal/model"
)

type Manager interface {
	TryAcquire(ctx context.Context, key, owner string, lease time.Duration, metadata Metadata) (bool, error)
	Refresh(ctx context.Context, key, owner string, lease time.Duration) error
	Release(ctx context.Context, key, owner string) error
}

type Metadata struct {
	RunID     string
	AttemptID string
	IssueID   string
	Stage     string
	WorkerID  string
}

type NoopManager struct{}

func (NoopManager) TryAcquire(context.Context, string, string, time.Duration, Metadata) (bool, error) {
	return true, nil
}

func (NoopManager) Refresh(context.Context, string, string, time.Duration) error {
	return nil
}

func (NoopManager) Release(context.Context, string, string) error {
	return nil
}

func KeysForStage(run model.RunRequest, spec model.StageSpec) []string {
	container := spec.ContainerConfig()
	keys := make([]string, 0, len(container.Permissions.RepoControl))
	for _, surface := range container.Permissions.RepoControl {
		switch model.StageSurface(surface) {
		case model.StageSurfaceComponents:
			if scope := run.RepoScope(); scope != "" {
				keys = append(keys, fmt.Sprintf("apprepo:%s", scope))
			}
		case model.StageSurfaceGitOps:
			target := run.EnvironmentTarget(spec.Environment())
			if target.GitOpsRepo.ProjectPath != "" && target.GitOpsRepo.Path != "" {
				keys = append(keys, fmt.Sprintf(
					"gitops:%s:%s:%s",
					target.GitOpsRepo.ProjectPath,
					target.GitOpsRepo.Environment,
					target.GitOpsRepo.Path,
				))
			}
		case model.StageSurfaceJournal:
			journal := run.DeliveryTarget().JournalOrDefault()
			if journal.Repo.ProjectPath != "" {
				keys = append(keys, fmt.Sprintf("journal:%s:%s", journal.Repo.ProjectPath, journal.Path))
			}
		}
	}
	return keys
}
