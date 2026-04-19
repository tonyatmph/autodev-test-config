package main

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"g7.mph.tech/mph-tech/autodev/internal/app"
	"g7.mph.tech/mph-tech/autodev/internal/model"
	"g7.mph.tech/mph-tech/autodev/internal/stagecontainer"
)

func TestRunLocalSmokeFixtureReachesApprovalGate(t *testing.T) {
	t.Skip("skipping local smoke test due to removal of python helpers")
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	installFixedConfigForLocalSmoke(t, filepath.Join(repoRoot, "TEST"))
	requireDockerAndStageImages(t, repoRoot)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(cwd)
	}()

	dataDir := t.TempDir()
	fixtureRoot := filepath.Join(t.TempDir(), "e2e-repos")
	configPath := writeTestConfig(
		t,
		repoRoot,
		dataDir,
		[]string{fixtureRoot},
		filepath.Join(fixtureRoot, "autodev-e2e-work-orders"),
	)

	cmd := exec.Command("bash", filepath.Join("hack", "init-e2e-fixture-repos.sh"))
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "AUTODEV_E2E_REPO_ROOT="+fixtureRoot)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("init local repos: %v\n%s", err, string(output))
	}

	issuePath := filepath.Join("hack", "local-sample-issue.json")
	secretsPath := filepath.Join("hack", "smoke-secrets.json")

	runMaterialize(configPath, []string{"--issue", issuePath})

	stdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	defer func() {
		os.Stdout = stdout
	}()

	done := make(chan []byte, 1)
	go func() {
		payload, _ := io.ReadAll(reader)
		done <- payload
	}()

	runLocal(configPath, []string{"--issue", issuePath, "--smoke-secrets", secretsPath})

	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output := <-done

	var state model.PersistedState
	if err := json.Unmarshal(output, &state); err != nil {
		t.Fatalf("decode local smoke output: %v\n%s", err, string(output))
	}

	run := firstRun(t, state)
	if run.Status == model.RunStatusPending {
		t.Fatalf("expected run to advance past pending, got %s", run.Status)
	}

	if commit, ok := run.Metadata["work_order_commit"].(string); !ok || commit == "" {
		t.Fatalf("expected work_order_commit metadata, got %v", run.Metadata["work_order_commit"])
	}
	attempts := attemptsForRun(state, run.ID)
	for _, stage := range []string{"intake", "plan", "implement"} {
		attempt, ok := attempts[stage]
		if !ok {
			t.Fatalf("expected attempt for %s, got %+v", stage, attempts)
		}
		if attempt.Status == model.AttemptStatusPending {
			t.Fatalf("expected %s to progress past pending, got %+v", stage, attempt)
		}
	}
}

func firstRun(t *testing.T, state model.PersistedState) model.RunRequest {
	t.Helper()
	for _, run := range state.Runs {
		return run
	}
	t.Fatal("expected at least one run")
	return model.RunRequest{}
}

func attemptsForRun(state model.PersistedState, runID string) map[string]model.StageAttempt {
	out := make(map[string]model.StageAttempt)
	for _, attempt := range state.Attempts {
		if attempt.RunID == runID {
			out[attempt.Stage] = attempt
		}
	}
	return out
}

func writeTestConfig(t *testing.T, rootDir, dataDir string, repoRoots []string, workOrderRepo string) string {
	t.Helper()
	cfg := struct {
		Paths   app.PathsConfig   `json:"paths"`
		GitLab  app.GitLabConfig  `json:"gitlab"`
		Stores  app.StoresConfig  `json:"stores"`
		Secrets app.SecretsConfig `json:"secrets"`
	}{
		Paths: app.PathsConfig{
			RootDir:       rootDir,
			DataDir:       dataDir,
			WorkOrderRepo: workOrderRepo,
			RepoRoots:     repoRoots,
			SmokeSecrets:  filepath.Join(rootDir, "hack", "smoke-secrets.json"),
		},
		GitLab: app.GitLabConfig{
			TokenName: "gitlab-token",
		},
		Stores: app.StoresConfig{
			RatchetPostgresDSN: "noop",
		},
		Secrets: app.SecretsConfig{
			LocalKeychainSvc: "autodev",
		},
	}
	path := filepath.Join(t.TempDir(), "autodev.test.json")
	payload, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func requireDockerAndStageImages(t *testing.T, root string) {
	t.Helper()
	if output, err := exec.Command("docker", "info").CombinedOutput(); err != nil {
		t.Fatalf("docker unavailable: %v\n%s", err, string(output))
	}
	image, err := stagecontainer.ResolveRuntimeImage("closeout")
	if err != nil {
		t.Fatalf("resolve runtime image: %v", err)
	}
	if output, err := exec.Command("docker", "image", "inspect", image.Ref).CombinedOutput(); err != nil {
		t.Fatalf("required stage image %s is missing; run `make build-stage-images` first: %v\n%s", image.Ref, err, string(output))
	}
}

func installFixedConfigForLocalSmoke(t *testing.T, sourceRoot string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	targetRoot := filepath.Join(home, ".autodev", "config")
	if err := copyTreeForLocalSmoke(sourceRoot, targetRoot); err != nil {
		t.Fatalf("install fixed config: %v", err)
	}
}

func copyTreeForLocalSmoke(src, dst string) error {
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
