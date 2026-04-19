package stagecontainer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"g7.mph.tech/mph-tech/autodev/internal/app"
	"g7.mph.tech/mph-tech/autodev/internal/contracts"
	"g7.mph.tech/mph-tech/autodev/internal/model"
)

func TestRealDockerContainerBoundaryOnlyWritesMountedPaths(t *testing.T) {
	repoRoot := repoRoot(t)
	image, err := ResolveRuntimeImage("plan")
	if err != nil {
		t.Fatalf("resolve plan runtime image: %v", err)
	}
	if output, err := exec.Command("docker", "image", "inspect", image.Ref).CombinedOutput(); err != nil {
		t.Fatalf("required stage image %s is missing; run `make build-stage-images` first: %v\n%s", image.Ref, err, string(output))
	}

	dataDir, err := os.MkdirTemp("", "autodev-stagecontainer-data-")
	if err != nil {
		t.Fatalf("create data dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dataDir) })
	workspace := filepath.Join(dataDir, "workspaces", "run-boundary", "plan")
	allowedPath := filepath.Join(workspace, "allowed.txt")
	repoPath := filepath.Join(dataDir, "repos", "run-boundary", "component-a")
	allowedRepoPath := filepath.Join(repoPath, "src", "allowed.txt")
	forbiddenRepoPath := filepath.Join(repoPath, "policy", "blocked.txt")
	workOrderRepo := filepath.Join(dataDir, "work-orders")
	forbiddenDir, err := os.MkdirTemp("", "autodev-stagecontainer-forbidden-")
	if err != nil {
		t.Fatalf("create forbidden dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(forbiddenDir) })
	forbiddenPath := filepath.Join(forbiddenDir, "blocked.txt")
	for _, path := range []string{workOrderRepo, filepath.Join(repoPath, "src"), filepath.Join(repoPath, "policy")} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("create test path %s: %v", path, err)
		}
	}
	if err := os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("repo\n"), 0o644); err != nil {
		t.Fatalf("seed repo root: %v", err)
	}
	if err := os.WriteFile(forbiddenRepoPath, []byte("original\n"), 0o644); err != nil {
		t.Fatalf("seed forbidden path: %v", err)
	}
	if err := os.MkdirAll(workOrderRepo, 0o755); err != nil {
		t.Fatalf("create work-order repo: %v", err)
	}
	probeScript := "echo 'allowed' > " + quotePy(allowedPath) + "\n" +
		"echo 'allowed-repo' > " + quotePy(allowedRepoPath) + "\n" +
		"if echo 'forbidden-repo' > " + quotePy(forbiddenRepoPath) + " 2>/dev/null; then exit 1; fi\n" +
		"mkdir -p $(dirname " + quotePy(forbiddenPath) + ") 2>/dev/null || true\n" +
		"if echo 'forbidden-host' > " + quotePy(forbiddenPath) + " 2>/dev/null; then exit 1; fi\n"

	if err := contracts.WriteFile(filepath.Join(workspace, "context.json"), contracts.StageContextSchema, map[string]any{
		"schema_version": "autodev-stage-context-v1",
		"stage": map[string]any{
			"name": "plan",
			"operation_plan": map[string]any{
				"steps": []map[string]any{
					{
						"name": "probe_mount_boundary",
						"command": []string{
							"sh", "-c",
							probeScript,
						},
					},
				},
			},
			"runtime": map[string]any{
				"summary": "boundary probe",
				"transitions": map[string]any{
					"on_success": []string{},
					"on_failure": []string{},
				},
				"success_criteria": map[string]any{
					"result_status":   "succeeded",
					"require_summary": true,
				},
			},
		},
		"run":        map[string]any{"id": "run-boundary"},
		"attempt":    map[string]any{"id": "attempt-boundary", "attempt": 1},
		"issue":      map[string]any{"id": "issue-boundary"},
		"work_order": map[string]any{"id": "wo-boundary"},
		"policy":     map[string]any{},
		"invariants": map[string]any{},
		"pipeline_contract": map[string]any{
			"stage_catalog":    []any{},
			"pipeline_catalog": map[string]any{},
			"runtime_images": map[string]any{
				"plan": map[string]any{"image_ref": image.Ref, "image_digest": image.Digest},
			},
		},
		"runtime_isolation": map[string]any{
			"writable": []map[string]any{
				{
					"root":      repoPath,
					"patterns":  []string{"src/**"},
					"allow_all": false,
				},
			},
		},
		"materialized_repos": []any{
			map[string]any{
				"type": "component",
				"path": repoPath,
			},
		},
		"paths": map[string]any{
			"work_order_repo": workOrderRepo,
		},
	}); err != nil {
		t.Fatalf("write stage context: %v", err)
	}

	spec := model.StageSpec{
		Name:       "plan",
		Entrypoint: []string{"autodev-stage-runtime"},
		Container: model.StageContainer{
			Permissions: model.StagePermissions{
				RuntimeUser: model.RuntimeUserSpec{
					Mode:          model.RuntimeIsolationModeContainer,
					ContainerUser: fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
				},
			},
		},
	}
	env := app.Env{
		DataDir:       dataDir,
		RootDir:       repoRoot,
		WorkOrderRepo: workOrderRepo,
	}
	runner := &Docker{}
	if err := runner.Run(context.Background(), Config{Env: env}, spec, "run-boundary", image); err != nil {
		t.Fatalf("run stage container: %v", err)
	}

	allowed, err := os.ReadFile(allowedPath)
	if err != nil {
		t.Fatalf("expected mounted workspace write to land on host: %v", err)
	}
	if strings.TrimSpace(string(allowed)) != "allowed" {
		t.Fatalf("unexpected mounted file contents: %q", string(allowed))
	}
	allowedRepo, err := os.ReadFile(allowedRepoPath)
	if err != nil {
		t.Fatalf("expected declared repo write to land on host: %v", err)
	}
	if strings.TrimSpace(string(allowedRepo)) != "allowed-repo" {
		t.Fatalf("unexpected repo writable path contents: %q", string(allowedRepo))
	}
	blockedRepo, err := os.ReadFile(forbiddenRepoPath)
	if err != nil {
		t.Fatalf("read forbidden repo path: %v", err)
	}
	if strings.TrimSpace(string(blockedRepo)) != "original" {
		t.Fatalf("expected undeclared repo path to remain unchanged, got %q", string(blockedRepo))
	}
	if _, err := os.Stat(forbiddenPath); !os.IsNotExist(err) {
		t.Fatalf("expected unmounted host path %s to remain untouched, err=%v", forbiddenPath, err)
	}
}

func TestRunnerImageBuildRequiresExplicitConfigSourceAndProducesDistinctConfigTrees(t *testing.T) {
	t.Skip("skipping due to new boot.sh architecture")
	repoRoot := repoRoot(t)
	prodImage := "autodev-config-proof:prod"
	testImage := "autodev-config-proof:test"
	controlProdImage := "autodev-config-proof-control:prod"
	controlTestImage := "autodev-config-proof-control:test"
	t.Cleanup(func() {
		_, _ = exec.Command("docker", "image", "rm", "-f", prodImage, testImage, controlProdImage, controlTestImage).CombinedOutput()
	})

	for _, tc := range []struct {
		dockerfile string
		source     string
		tag        string
	}{
		{dockerfile: filepath.Join(repoRoot, "docker", "runner", "Dockerfile"), source: "PROD", tag: prodImage},
		{dockerfile: filepath.Join(repoRoot, "docker", "runner", "Dockerfile"), source: "TEST", tag: testImage},
		{dockerfile: filepath.Join(repoRoot, "docker", "control-plane", "Dockerfile"), source: "PROD", tag: controlProdImage},
		{dockerfile: filepath.Join(repoRoot, "docker", "control-plane", "Dockerfile"), source: "TEST", tag: controlTestImage},
	} {
		if output, err := exec.Command("docker", "build", "-t", tc.tag+"-missing", "-f", tc.dockerfile, repoRoot).CombinedOutput(); err == nil {
			t.Fatalf("expected docker build without CONFIG_SOURCE to fail for %s", tc.dockerfile)
		} else if !strings.Contains(string(output), "CONFIG_SOURCE must be PROD or TEST") {
			t.Fatalf("expected missing CONFIG_SOURCE failure for %s, got:\n%s", tc.dockerfile, string(output))
		}
		output, err := exec.Command(
			"docker", "build",
			"--build-arg", "CONFIG_SOURCE="+tc.source,
			"-t", tc.tag,
			"-f", tc.dockerfile,
			repoRoot,
		).CombinedOutput()
		if err != nil {
			t.Fatalf("build %s image for %s: %v\n%s", tc.source, tc.dockerfile, err, string(output))
		}
	}

	readSource := func(image string) string {
		cmd := exec.Command("docker", "run", "--rm", "--entrypoint", "cat", image, "/home/autodev/.autodev/config/source.json")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("read source marker from %s: %v\n%s", image, err, string(output))
		}
		var payload struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(output, &payload); err != nil {
			t.Fatalf("decode source marker from %s: %v\n%s", image, err, string(output))
		}
		return payload.Name
	}

	if got := readSource(prodImage); got != "PROD" {
		t.Fatalf("expected PROD image source marker, got %q", got)
	}
	if got := readSource(testImage); got != "TEST" {
		t.Fatalf("expected TEST image source marker, got %q", got)
	}
	if got := readSource(controlProdImage); got != "PROD" {
		t.Fatalf("expected control-plane PROD image source marker, got %q", got)
	}
	if got := readSource(controlTestImage); got != "TEST" {
		t.Fatalf("expected control-plane TEST image source marker, got %q", got)
	}
}

func TestComposeRequiresExplicitConfigSourceAndDoesNotMountMutableRuntimeSources(t *testing.T) {
	t.Skip("skipping due to new boot.sh architecture")
	repoRoot := repoRoot(t)
	composeFile := filepath.Join(repoRoot, "docker-compose.yml")

	cmd := exec.Command("docker", "compose", "-f", composeFile, "config")
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected docker compose config without CONFIG_SOURCE to fail")
	}
	if !strings.Contains(string(output), "CONFIG_SOURCE must be set to PROD or TEST") {
		t.Fatalf("expected missing CONFIG_SOURCE failure, got:\n%s", string(output))
	}

	cmd = exec.Command("docker", "compose", "-f", composeFile, "config")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "CONFIG_SOURCE=PROD")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("docker compose config with CONFIG_SOURCE=PROD: %v\n%s", err, string(output))
	}
	rendered := string(output)
	if strings.Contains(rendered, "./stage-specs:") || strings.Contains(rendered, "./tooling:") || strings.Contains(rendered, "./prompts:") || strings.Contains(rendered, "./hack:") {
		t.Fatalf("expected compose config to avoid mutable runtime source mounts, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "CONFIG_SOURCE: PROD") {
		t.Fatalf("expected compose config to propagate CONFIG_SOURCE build arg, got:\n%s", rendered)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return root
}

func quotePy(path string) string {
	return "'" + strings.ReplaceAll(path, "'", "\\'") + "'"
}
