package runner

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"g7.mph.tech/mph-tech/autodev/internal/isolation"
	"g7.mph.tech/mph-tech/autodev/internal/model"
)

func (e *StageExecutor) initializeStageRepos(ctx context.Context, spec model.StageSpec, run model.RunRequest, workOrder model.WorkOrder, sandbox *isolation.Sandbox, output map[string]any) error {
	repos := make([]map[string]any, 0, len(workOrder.Delivery.SelectedDeliveryComponents())+2)
	if spec.MaterializedSurface(model.StageSurfaceComponents) {
		for _, component := range workOrder.Delivery.SelectedDeliveryComponents() {
			repoDir, status, err := e.repos.EnsureRunRepo(ctx, run.ID, component.Repo)
			if err != nil {
				return fmt.Errorf("materialize component repo %s: %w", component.Repo.ProjectPath, err)
			}
			if repoDir != "" {
				e.grantComponentRepoAccess(sandbox, repoDir, component)
			}
			repos = append(repos, map[string]any{
				"type":         "component",
				"name":         component.Name,
				"project_path": component.Repo.ProjectPath,
				"path":         repoDir,
				"status":       status,
			})
		}
	}

	if spec.MaterializedSurface(model.StageSurfaceJournal) {
		target := workOrder.Delivery.JournalOrDefault()
		if target.Repo.ProjectPath != "" {
			repoDir, status, err := e.repos.EnsureRunRepo(ctx, run.ID, target.Repo)
			if err != nil {
				return fmt.Errorf("materialize journal repo %s: %w", target.Repo.ProjectPath, err)
			}
			if repoDir != "" {
				sandbox.GrantRepoControl(repoDir, "run-scoped journal repo")
				journalPattern := filepath.ToSlash(strings.Trim(target.Path, "/")) + "/**"
				if strings.TrimSpace(target.Path) == "" {
					journalPattern = "**"
				}
				sandbox.GrantPatternAccess(repoDir, []string{journalPattern}, "run-scoped journal repo")
				if spec.WriteAsIdentity() == model.ExecutionIdentityGenerator {
					generatedPattern := filepath.ToSlash(filepath.Join(strings.Trim(target.Path, "/"), "generated")) + "/**"
					sandbox.GrantPatternAccess(repoDir, []string{generatedPattern}, "run-scoped generator outputs")
				}
			}
			repos = append(repos, map[string]any{
				"type":         "journal",
				"project_path": target.Repo.ProjectPath,
				"path":         repoDir,
				"status":       status,
			})
		}
	}

	if spec.MaterializedSurface(model.StageSurfaceGitOps) {
		envName := stageEnvironment(spec)
		if envName == "" {
			return fmt.Errorf("stage %s requested gitops materialization without runtime.environment", spec.Name)
		}
		target := run.EnvironmentTarget(envName)
		repo := model.RepoTarget{
			ProjectPath:         target.GitOpsRepo.ProjectPath,
			DefaultBranch:       target.GitOpsRepo.PromotionBranch,
			WorkingBranchPrefix: "autodev",
			Ref:                 target.GitOpsRepo.Ref,
			MaterializationPath: target.GitOpsRepo.MaterializationPath,
		}
		repoDir, status, err := e.repos.EnsureRunRepo(ctx, run.ID, repo)
		if err != nil {
			return fmt.Errorf("materialize gitops repo %s: %w", target.GitOpsRepo.ProjectPath, err)
		}
		if repoDir != "" {
			sandbox.GrantRepoControl(repoDir, fmt.Sprintf("run-scoped gitops repo %s", envName))
			gitopsPattern := filepath.ToSlash(strings.Trim(target.GitOpsRepo.Path, "/")) + "/**"
			sandbox.GrantPatternAccess(repoDir, []string{gitopsPattern}, fmt.Sprintf("run-scoped gitops repo %s", envName))
		}
		repos = append(repos, map[string]any{
			"type":         "gitops",
			"environment":  envName,
			"project_path": target.GitOpsRepo.ProjectPath,
			"path":         repoDir,
			"status":       status,
		})
	}

	output["materialized_repos"] = repos
	return nil
}

func stageEnvironment(spec model.StageSpec) string {
	return spec.Environment()
}

func (e *StageExecutor) grantComponentRepoAccess(sandbox *isolation.Sandbox, repoDir string, component model.DeliveryComponent) {
	if repoDir == "" {
		return
	}
	patterns := mutablePatterns(component.OwnershipRulesFor(sandbox.WriteAs))
	if len(patterns) > 0 && sandbox.Permissions.AllowsWritable(model.StageSurfaceComponents) {
		sandbox.GrantPatternAccess(repoDir, patterns, fmt.Sprintf("%s component %s", sandbox.WriteAs, component.Name))
	}
	if sandbox.Permissions.AllowsRepoControl(model.StageSurfaceComponents) {
		sandbox.GrantRepoControl(repoDir, fmt.Sprintf("%s component %s", sandbox.WriteAs, component.Name))
	}
}

func mutablePatterns(rules []model.PathOwnershipRule) []string {
	patterns := make([]string, 0, len(rules))
	for _, rule := range rules {
		if !rule.Mutable {
			continue
		}
		patterns = append(patterns, rule.Paths...)
	}
	return patterns
}
