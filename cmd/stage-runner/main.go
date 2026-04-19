package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"g7.mph.tech/mph-tech/autodev/internal/app"
	"g7.mph.tech/mph-tech/autodev/internal/artifacts"
	"g7.mph.tech/mph-tech/autodev/internal/contracts"
	"g7.mph.tech/mph-tech/autodev/internal/configsource"
	"g7.mph.tech/mph-tech/autodev/internal/controlplane"
	"g7.mph.tech/mph-tech/autodev/internal/gitlab"
	"g7.mph.tech/mph-tech/autodev/internal/httpapi"
	"g7.mph.tech/mph-tech/autodev/internal/locks"
	"g7.mph.tech/mph-tech/autodev/internal/model"
	"g7.mph.tech/mph-tech/autodev/internal/ratchet"
	"g7.mph.tech/mph-tech/autodev/internal/runner"
	"g7.mph.tech/mph-tech/autodev/internal/secrets"
	"g7.mph.tech/mph-tech/autodev/internal/signals"
	"g7.mph.tech/mph-tech/autodev/internal/store"
)

func main() {
	configPath, args := extractConfigPath(os.Args[1:])
	if len(args) < 1 {
		usage()
	}
	switch args[0] {
	case "worker":
		runWorker(configPath, args[1:])
	case "materialize":
		runMaterialize(configPath, args[1:])
	case "run":
		runSingle(configPath, args[1:])
	case "local":
		runLocal(configPath, args[1:])
	default:
		usage()
	}
}

func runWorker(configPath string, args []string) {
	fs := flag.NewFlagSet("worker", flag.ExitOnError)
	controlPlaneURL := fs.String("control-plane-url", "http://127.0.0.1:8080", "control plane url")
	workerID := fs.String("worker-id", "worker-1", "worker id")
	leaseSeconds := fs.Int("lease-seconds", 30, "lease duration in seconds")
	once := fs.Bool("once", false, "run only one claim loop")
	stageList := fs.String("stages", "", "comma-separated stage allow list")
	fs.Parse(args)

	env := mustLoadConfig(configPath)
	specs := mustLoadSpecs()
	pipelineCatalog := mustLoadPipelineCatalog()
	executor := runner.New(env.RootDir, env.DataDir, artifacts.New(env.ArtifactDir), secrets.NewDefaultProvider(env), ratchetRanker(env), runner.WithRepoRoots(env.RepoRoots), runner.WithWorkOrderRepo(env.WorkOrderRepo), runner.WithGitLab(newGitLabAdapter(env)), runner.WithPipelineContract(specs, pipelineCatalog), runner.WithExecutionEnv(env))
	client := httpapi.NewClient(*controlPlaneURL)
	allowedStages := splitCSV(*stageList)

	for {
		attempt, spec, err := client.Claim(*workerID, *leaseSeconds, allowedStages)
		if err != nil {
			log.Fatal(err)
		}
		if attempt == nil {
			if *once {
				return
			}
			time.Sleep(2 * time.Second)
			continue
		}

		state, err := client.Snapshot()
		if err != nil {
			log.Fatal(err)
		}
		run := state.Runs[attempt.RunID]
		issue := state.Issues[run.IssueID].DeliveryIssue
		_ = client.Heartbeat(attempt.ID, *leaseSeconds, "execution started", map[string]any{
			"current_state": "STARTING",
		})
		result, artifactsOut, err := executor.ExecuteWithProgress(context.Background(), spec, run, *attempt, issue, func(summary string, metadata map[string]any) {
			_ = client.Heartbeat(attempt.ID, *leaseSeconds, summary, metadata)
		})
		if err != nil {
			result = model.StageResult{
				Status:  model.AttemptStatusFailed,
				Summary: err.Error(),
			}
		}
		if err := client.Complete(attempt.ID, result, artifactsOut); err != nil {
			log.Fatal(err)
		}
		if err := client.Reconcile(); err != nil {
			log.Fatal(err)
		}
		if *once {
			return
		}
	}
}

func runSingle(configPath string, args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	stageName := fs.String("stage", "", "stage name")
	runID := fs.String("run-id", "", "run id")
	fs.Parse(args)
	if *stageName == "" || *runID == "" {
		log.Fatal("run requires --stage and --run-id")
	}

	env := mustLoadConfig(configPath)
	specs := mustLoadSpecs()
	specMap := specMapByName(specs)
	spec, ok := specMap[*stageName]
	if !ok {
		log.Fatalf("unknown stage %s", *stageName)
	}

	service := controlplane.New(
		store.New(env.StateDir),
		newGitLabAdapter(env),
		mustLocker(env),
		signaler(env),
		specs,
		env.WorkOrderRepo,
	)
	state, err := service.Snapshot()
	if err != nil {
		log.Fatal(err)
	}
	run, ok := state.Runs[*runID]
	if !ok {
		log.Fatalf("run %s not found", *runID)
	}
	attempt := findAttempt(state, run.ID, *stageName)
	if attempt == nil {
		log.Fatalf("no attempt for run %s stage %s", *runID, *stageName)
	}
	issue := state.Issues[run.IssueID].DeliveryIssue
	pipelineCatalog := mustLoadPipelineCatalog()
	executor := runner.New(env.RootDir, env.DataDir, artifacts.New(env.ArtifactDir), singleRunSecretProvider(env), ratchetRanker(env), runner.WithRepoRoots(env.RepoRoots), runner.WithWorkOrderRepo(env.WorkOrderRepo), runner.WithGitLab(newGitLabAdapter(env)), runner.WithPipelineContract(specs, pipelineCatalog), runner.WithExecutionEnv(env))
	result, artifactsOut, err := executor.ExecuteWithProgress(context.Background(), spec, run, *attempt, issue, func(summary string, metadata map[string]any) {
		_ = service.Heartbeat(attempt.ID, 30*time.Second, summary, metadata)
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := service.Complete(attempt.ID, result, artifactsOut); err != nil {
		log.Fatal(err)
	}
	if err := service.Reconcile(); err != nil {
		log.Fatal(err)
	}
}

func runLocal(configPath string, args []string) {
	fs := flag.NewFlagSet("local", flag.ExitOnError)
	issuePath := fs.String("issue", "hack/sample-issue.json", "path to issue json")
	issueID := fs.String("issue-id", "", "canonical GitLab issue id for real-GitLab local runs")
	smokeSecretsPath := fs.String("smoke-secrets", "hack/smoke-secrets.json", "path to local smoke secrets fixture")
	fs.Parse(args)

	env := mustLoadConfig(configPath)
	issue := materializeIssueInput(env, *issuePath, *issueID)

	specs := mustLoadSpecs()
	service := controlplane.New(
		store.New(env.StateDir),
		newGitLabAdapter(env),
		mustLocker(env),
		signaler(env),
		specs,
		env.WorkOrderRepo,
	)
	must(service.EnqueueFromGitLab())

	if strings.TrimSpace(*issueID) != "" {
		state, err := service.Snapshot()
		must(err)
		tracked, ok := state.Issues[*issueID]
		if !ok {
			log.Fatalf("issue %s was not found after GitLab enqueue", *issueID)
		}
		issue = tracked.DeliveryIssue
	}
	must(service.ReconcileIssue(issue.ID))
	state, err := service.Snapshot()
	must(err)
	if latestRunForIssueID(state, issue.ID) == nil {
		materializeRef := fmt.Sprintf("--issue %s", *issuePath)
		if strings.TrimSpace(*issueID) != "" {
			materializeRef = fmt.Sprintf("--issue-id %s", *issueID)
		}
		log.Fatalf("issue %s has no materialized run; run `stage-runner --config %s materialize %s` first", issue.ID, configPath, materializeRef)
	}

	secretProvider := localSecretProvider(env, *smokeSecretsPath)
	env.SmokeSecretsPath = *smokeSecretsPath
	pipelineCatalog := mustLoadPipelineCatalog()
	executor := runner.New(env.RootDir, env.DataDir, artifacts.New(env.ArtifactDir), secretProvider, ratchetRanker(env), runner.WithRepoRoots(env.RepoRoots), runner.WithWorkOrderRepo(env.WorkOrderRepo), runner.WithGitLab(newGitLabAdapter(env)), runner.WithPipelineContract(specs, pipelineCatalog), runner.WithExecutionEnv(env))
	for i := 0; i < 64; i++ {
		state, err := service.Snapshot()
		must(err)
		if done(state, issue.ID) {
			break
		}

		var attempt *model.StageAttempt
		var spec model.StageSpec
		attempt, spec, err = service.ClaimNextForIssue("local-worker", 30*time.Second, nil, issue.ID)
		must(err)
		if attempt == nil {
			break
		}
		run := state.Runs[attempt.RunID]
		result, artifactsOut, err := executor.ExecuteWithProgress(context.Background(), spec, run, *attempt, issue, func(summary string, metadata map[string]any) {
			_ = service.Heartbeat(attempt.ID, 30*time.Second, summary, metadata)
		})
		if err != nil {
			result = model.StageResult{
				Status:  model.AttemptStatusFailed,
				Summary: err.Error(),
			}
		}
		must(service.Complete(attempt.ID, result, artifactsOut))
		must(service.ReconcileIssue(issue.ID))
	}

	state, err = service.Snapshot()
	must(err)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	must(enc.Encode(state))
}

func runMaterialize(configPath string, args []string) {
	fs := flag.NewFlagSet("materialize", flag.ExitOnError)
	issuePath := fs.String("issue", "hack/sample-issue.json", "path to issue json")
	issueID := fs.String("issue-id", "", "canonical GitLab issue id for real-GitLab materialization")
	fs.Parse(args)

	env := mustLoadConfig(configPath)
	issue := materializeIssueInput(env, *issuePath, *issueID)
	specs := mustLoadSpecs()
	service := controlplane.New(
		store.New(env.StateDir),
		newGitLabAdapter(env),
		mustLocker(env),
		signaler(env),
		specs,
		env.WorkOrderRepo,
	)
	must(service.EnqueueFromGitLab())
	if strings.TrimSpace(*issueID) != "" {
		state, err := service.Snapshot()
		must(err)
		tracked, ok := state.Issues[*issueID]
		if !ok {
			log.Fatalf("issue %s was not found after GitLab enqueue", *issueID)
		}
		issue = tracked.DeliveryIssue
	}
	run, err := service.EnsureRunForIssue(issue.ID)
	must(err)
	pipelineCatalog := mustLoadPipelineCatalog()
	outputs, err := runner.MaterializePipelineArtifacts(context.Background(), env, specs, pipelineCatalog, run, issue)
	must(err)
	run, err = service.PersistPipelineArtifacts(run.ID, outputs)
	must(err)

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	must(enc.Encode(map[string]any{
		"issue_id": issue.ID,
		"run_id":   run.ID,
		"outputs":  outputs,
	}))
}

func localSecretProvider(env app.Env, smokeSecretsPath string) secrets.Provider {
	if strings.TrimSpace(smokeSecretsPath) == "" {
		return secrets.NewDefaultProvider(env)
	}
	provider, err := secrets.NewFileProvider(smokeSecretsPath)
	if err != nil {
		log.Fatalf("load smoke secrets fixture %q: %v", smokeSecretsPath, err)
	}
	return secrets.NewChain(provider, secrets.NewDefaultProvider(env))
}

func singleRunSecretProvider(env app.Env) secrets.Provider {
	if strings.TrimSpace(env.SmokeSecretsPath) != "" {
		return localSecretProvider(env, env.SmokeSecretsPath)
	}
	return secrets.NewDefaultProvider(env)
}

func materializeIssueInput(env app.Env, issuePath, explicitIssueID string) model.DeliveryIssue {
	if strings.TrimSpace(explicitIssueID) != "" {
		return model.DeliveryIssue{ID: explicitIssueID}
	}
	if err := os.MkdirAll(filepath.Join(env.GitLabDir, "issues"), 0o755); err != nil {
		log.Fatal(err)
	}
	data, err := os.ReadFile(issuePath)
	if err != nil {
		log.Fatal(err)
	}
	if err := contracts.Validate(contracts.IssueFileSchema, issuePath, data); err != nil {
		log.Fatal(err)
	}
	var issue model.DeliveryIssue
	if err := json.Unmarshal(data, &issue); err != nil {
		log.Fatal(err)
	}
	dest := filepath.Join(env.GitLabDir, "issues", issue.ID+".json")
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		log.Fatal(err)
	}
	return issue
}

func mustLoadConfig(path string) app.Env {
	env, err := app.Load(path)
	if err != nil {
		log.Fatal(err)
	}
	return env
}

func mustLoadSpecs() []model.StageSpec {
	specs, err := configsource.LoadStageSpecs()
	if err != nil {
		log.Fatal(err)
	}
	return specs
}

func mustLoadPipelineCatalog() map[string]any {
	payload, err := configsource.LoadPipelineCatalog()
	if err != nil {
		log.Fatal(err)
	}
	if len(payload) == 0 {
		log.Fatal("embedded pipeline catalog is empty")
	}
	return payload
}

func specMapByName(specs []model.StageSpec) map[string]model.StageSpec {
	out := make(map[string]model.StageSpec, len(specs))
	for _, spec := range specs {
		out[spec.Name] = spec
	}
	return out
}

func extractConfigPath(args []string) (string, []string) {
	configPath := ""
	out := make([]string, 0, len(args))
	skip := false
	for i, arg := range args {
		if skip {
			skip = false
			continue
		}
		switch {
		case arg == "--config":
			if i+1 >= len(args) {
				log.Fatal("--config requires a path")
			}
			configPath = args[i+1]
			skip = true
		case strings.HasPrefix(arg, "--config="):
			configPath = strings.TrimPrefix(arg, "--config=")
		default:
			out = append(out, arg)
		}
	}
	if strings.TrimSpace(configPath) == "" {
		log.Fatal("--config is required")
	}
	return configPath, out
}

func done(state model.PersistedState, issueID string) bool {
	run := latestRunForIssueID(state, issueID)
	if run == nil {
		return false
	}
	return run.Status == model.RunStatusCompleted || run.Status == model.RunStatusAwaitingApproval || run.Status == model.RunStatusFailed
}

func findAttempt(state model.PersistedState, runID, stage string) *model.StageAttempt {
	var selected *model.StageAttempt
	for _, attempt := range state.Attempts {
		if attempt.RunID != runID || attempt.Stage != stage {
			continue
		}
		copyAttempt := attempt
		if selected == nil || copyAttempt.Attempt > selected.Attempt {
			selected = &copyAttempt
		}
	}
	return selected
}

func splitCSV(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func latestRunForIssueID(state model.PersistedState, issueID string) *model.RunRequest {
	var latest *model.RunRequest
	for _, run := range state.Runs {
		if run.IssueID != issueID {
			continue
		}
		copyRun := run
		if latest == nil || copyRun.CreatedAt.After(latest.CreatedAt) {
			latest = &copyRun
		}
	}
	return latest
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func mustLocker(env app.Env) locks.Manager {
	if env.LocksPostgresDSN == "" {
		return locks.NoopManager{}
	}
	locker, err := locks.NewPostgresManager(context.Background(), env.LocksPostgresDSN)
	if err != nil {
		log.Fatal(err)
	}
	return locker
}

func ratchetRanker(env app.Env) *ratchet.Service {
	if env.RatchetPostgresDSN == "noop" {
		return ratchet.NewService(ratchet.NoopStore{})
	}
	if env.RatchetPostgresDSN == "" {
		log.Fatal("stores.ratchet_postgres_dsn must be set explicitly; use \"noop\" only if that is the declared contract")
	}
	store, err := ratchet.NewPostgresStore(context.Background(), env.RatchetPostgresDSN)
	if err != nil {
		log.Fatal(err)
	}
	return ratchet.NewService(store)
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: stage-runner --config <path> <worker|materialize|run|local>")
	os.Exit(2)
}

func signaler(env app.Env) signals.Emitter {
	if env.SignalsPostgresDSN == "" {
		return signals.NewService(signals.NoopStore{})
	}
	store, err := signals.NewPostgresStore(context.Background(), env.SignalsPostgresDSN)
	if err != nil {
		log.Fatal(err)
	}
	return signals.NewService(store)
}

func newGitLabAdapter(env app.Env) gitlab.Adapter {
	if env.GitLabBaseURL == "" || env.GitLabIssuesProject == "" {
		return gitlab.NewFilesystemAdapter(env.GitLabDir)
	}
	token := strings.TrimSpace(env.GitLabToken)
	if token == "" {
		value, err := secrets.KeychainProvider{Service: env.LocalKeychainSvc}.Resolve(context.Background(), env.GitLabTokenName)
		if err != nil {
			log.Fatalf("resolve gitlab token %q: %v", env.GitLabTokenName, err)
		}
		token = value.Value
	}
	return gitlab.NewAPIAdapter(env.GitLabBaseURL, token, env.GitLabIssuesProject)
}
