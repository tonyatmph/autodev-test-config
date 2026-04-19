package locks

import (
	"testing"

	"g7.mph.tech/mph-tech/autodev/internal/model"
)

func TestKeysForStage(t *testing.T) {
	run := model.RunRequest{
		ID:      "run-1",
		IssueID: "issue-1",
		WorkOrder: model.WorkOrder{
			Delivery: model.DeliveryTarget{
				ApplicationRepo: model.RepoTarget{
					ProjectPath: "group/app",
				},
				Environments: model.PromotionTargets{
					Prod: model.EnvironmentTarget{
						Name: "prod",
						GitOpsRepo: model.GitOpsTarget{
							ProjectPath: "group/gitops",
							Environment: "prod",
							Path:        "clusters/prod/app",
						},
					},
				},
			},
		},
	}

	implementKeys := KeysForStage(run, model.StageSpec{
		Name: "implement",
		Container: model.StageContainer{
			Permissions: model.StagePermissions{
				RepoControl: []string{string(model.StageSurfaceComponents)},
			},
		},
	})
	if len(implementKeys) != 1 || implementKeys[0] != "apprepo:group/app" {
		t.Fatalf("unexpected implement keys: %#v", implementKeys)
	}

	gitopsKeys := KeysForStage(run, model.StageSpec{
		Name: "promote_prod",
		Runtime: model.StageRuntime{
			Environment: "prod",
		},
		Container: model.StageContainer{
			Permissions: model.StagePermissions{
				RepoControl: []string{string(model.StageSurfaceGitOps)},
			},
		},
	})
	if len(gitopsKeys) != 1 || gitopsKeys[0] != "gitops:group/gitops:prod:clusters/prod/app" {
		t.Fatalf("unexpected promote_prod keys: %#v", gitopsKeys)
	}

	rollbackKeys := KeysForStage(run, model.StageSpec{
		Name: "rollback_prod",
		Runtime: model.StageRuntime{
			Environment: "prod",
		},
		Container: model.StageContainer{
			Permissions: model.StagePermissions{
				RepoControl: []string{string(model.StageSurfaceGitOps)},
			},
		},
	})
	if len(rollbackKeys) != 1 || rollbackKeys[0] != "gitops:group/gitops:prod:clusters/prod/app" {
		t.Fatalf("unexpected rollback_prod keys: %#v", rollbackKeys)
	}
}
