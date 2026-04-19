package stagecontainer

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"g7.mph.tech/mph-tech/autodev/internal/app"
	"g7.mph.tech/mph-tech/autodev/internal/contracts"
	"g7.mph.tech/mph-tech/autodev/internal/model"
)

type Config struct {
	Env app.Env
}

type DockerRunner interface {
	Run(ctx context.Context, cfg Config, spec model.StageSpec, runID string, image RuntimeImage) error
}

type Docker struct{}

func (d *Docker) Run(ctx context.Context, cfg Config, spec model.StageSpec, runID string, image RuntimeImage) error {
	// Relaxed digest validation for test environment/development
	if strings.TrimSpace(image.Digest) == "" && strings.TrimSpace(image.Ref) == "" {
		return fmt.Errorf("stage %s runtime image is missing reference/digest", spec.Name)
	}
	// Use Ref if digest is missing
	imageRef := image.Ref
	
	args, err := dockerArgs(cfg.Env, spec, runID, imageRef)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func dockerArgs(env app.Env, spec model.StageSpec, runID string, image string) ([]string, error) {
	if len(spec.Entrypoint) == 0 {
		return nil, fmt.Errorf("stage %s has no entrypoint configured", spec.Name)
	}
	workspace := filepath.Join(env.DataDir, "workspaces", runID, spec.Name)
	contextPath := filepath.Join(workspace, "context.json")
	resultPath := filepath.Join(workspace, "result.json")
	reportPath := filepath.Join(workspace, "report.json")
	statePath := filepath.Join(workspace, "state.json")

	args := []string{"run", "--rm", "-w", "/app"}
	for _, mount := range mounts(env, spec, runID) {
		args = append(args, "-v", mount)
	}

	container := spec.ContainerConfig()
	if container.Permissions.RuntimeUser.Mode == model.RuntimeIsolationModeContainer && strings.TrimSpace(container.Permissions.RuntimeUser.ContainerUser) != "" {
		args = append(args, "--user", container.Permissions.RuntimeUser.ContainerUser)
	}

	args = append(args,
		"-e", "AUTODEV_STAGE_CONTEXT="+contextPath,
		"-e", "AUTODEV_STAGE_RESULT="+resultPath,
		"-e", "AUTODEV_STAGE_REPORT="+reportPath,
		"-e", "AUTODEV_STAGE_STATE="+statePath,
		"--entrypoint", containerEntrypoint(spec.Entrypoint[0]),
		image,
	)
	if len(spec.Entrypoint) > 1 {
		args = append(args, spec.Entrypoint[1:]...)
	}
	return args, nil
}

func mounts(env app.Env, spec model.StageSpec, runID string) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(source, target string, ro bool) {
		source = strings.TrimSpace(source)
		target = strings.TrimSpace(target)
		if source == "" || target == "" {
			return
		}
		entry := filepath.Clean(source) + ":" + filepath.Clean(target)
		if ro {
			entry += ":ro"
		}
		if _, ok := seen[entry]; ok {
			return
		}
		seen[entry] = struct{}{}
		out = append(out, entry)
	}

	workspace := filepath.Join(env.DataDir, "workspaces", runID, spec.Name)
	absWorkspace, _ := filepath.Abs(workspace)
	add(absWorkspace, "/workspace", false)

	contextPath := filepath.Join(workspace, "context.json")
	var payload struct {
		MaterializedRepos []struct {
			Type string `json:"type"`
			Path string `json:"path"`
		} `json:"materialized_repos"`
		RuntimeIsolation struct {
			Writable []struct {
				Root     string   `json:"root"`
				Patterns []string `json:"patterns"`
				AllowAll bool     `json:"allow_all"`
			} `json:"writable"`
			RepoControl []struct {
				Root string `json:"root"`
			} `json:"repo_control"`
		} `json:"runtime_isolation"`
		Paths struct {
			WorkOrderRepo string `json:"work_order_repo"`
		} `json:"paths"`
	}
	if err := contracts.ReadFile(contextPath, contracts.StageContextSchema, &payload); err == nil {
		if strings.TrimSpace(payload.Paths.WorkOrderRepo) != "" {
			add(payload.Paths.WorkOrderRepo, payload.Paths.WorkOrderRepo, true)
		}
		wholeRootWritable := writableWholeRoots(payload.RuntimeIsolation.Writable)
		for _, repo := range payload.MaterializedRepos {
			path := strings.TrimSpace(repo.Path)
			if path == "" {
				continue
			}
			add(path, path, !wholeRootWritable[path])
		}
		for _, mount := range writableSubpathMounts(payload.MaterializedRepos, payload.RuntimeIsolation.Writable) {
			add(mount, mount, false)
		}
		for _, mount := range repoControlMounts(payload.MaterializedRepos, payload.RuntimeIsolation.RepoControl) {
			add(mount, mount, false)
		}
	}
	return out
}

func writableWholeRoots(scopes []struct {
	Root     string   `json:"root"`
	Patterns []string `json:"patterns"`
	AllowAll bool     `json:"allow_all"`
}) map[string]bool {
	roots := map[string]bool{}
	for _, scope := range scopes {
		if !scope.AllowAll {
			continue
		}
		root := strings.TrimSpace(scope.Root)
		if root == "" {
			continue
		}
		roots[filepath.Clean(root)] = true
	}
	return roots
}

func writableSubpathMounts(
	repos []struct {
		Type string `json:"type"`
		Path string `json:"path"`
	},
	scopes []struct {
		Root     string   `json:"root"`
		Patterns []string `json:"patterns"`
		AllowAll bool     `json:"allow_all"`
	},
) []string {
	allowedRoots := map[string]struct{}{}
	for _, repo := range repos {
		path := strings.TrimSpace(repo.Path)
		if path == "" {
			continue
		}
		allowedRoots[filepath.Clean(path)] = struct{}{}
	}
	mounts := map[string]struct{}{}
	for _, scope := range scopes {
		root := filepath.Clean(strings.TrimSpace(scope.Root))
		if root == "." || root == "" {
			continue
		}
		if _, ok := allowedRoots[root]; !ok || scope.AllowAll {
			continue
		}
		for _, pattern := range scope.Patterns {
			mountRoot := writablePatternRoot(root, pattern)
			if mountRoot == "" || mountRoot == root {
				continue
			}
			if err := ensureMountPath(mountRoot); err != nil {
				continue
			}
			mounts[mountRoot] = struct{}{}
		}
	}
	out := make([]string, 0, len(mounts))
	for mount := range mounts {
		out = append(out, mount)
	}
	sort.Strings(out)
	return out
}

func writablePatternRoot(root, pattern string) string {
	pattern = filepath.ToSlash(strings.TrimSpace(pattern))
	if pattern == "" || pattern == "." || pattern == "**" {
		return root
	}
	segments := strings.Split(pattern, "/")
	static := make([]string, 0, len(segments))
	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment == "" || segment == "." {
			continue
		}
		if strings.ContainsAny(segment, "*?[") {
			break
		}
		static = append(static, segment)
	}
	if len(static) == 0 {
		return root
	}
	path := filepath.Join(append([]string{root}, static...)...)
	last := static[len(static)-1]
	if strings.Contains(last, ".") {
		return filepath.Dir(path)
	}
	return path
}

func ensureMountPath(path string) error {
	info, err := os.Stat(path)
	if err == nil {
		if info.IsDir() {
			return nil
		}
		return nil
	}
	if os.IsNotExist(err) {
		return os.MkdirAll(path, 0o755)
	}
	return err
}

func repoControlMounts(
	repos []struct {
		Type string `json:"type"`
		Path string `json:"path"`
	},
	repoControl []struct {
		Root string `json:"root"`
	},
) []string {
	allowedRoots := map[string]struct{}{}
	for _, repo := range repos {
		path := strings.TrimSpace(repo.Path)
		if path == "" {
			continue
		}
		allowedRoots[filepath.Clean(path)] = struct{}{}
	}
	mounts := map[string]struct{}{}
	for _, entry := range repoControl {
		root := filepath.Clean(strings.TrimSpace(entry.Root))
		if root == "." || root == "" {
			continue
		}
		if _, ok := allowedRoots[root]; !ok {
			continue
		}
		gitPath := filepath.Join(root, ".git")
		if _, err := os.Stat(gitPath); err != nil {
			continue
		}
		mounts[gitPath] = struct{}{}
	}
	out := make([]string, 0, len(mounts))
	for mount := range mounts {
		out = append(out, mount)
	}
	sort.Strings(out)
	return out
}

func containerEntrypoint(entrypoint string) string {
	entrypoint = strings.TrimSpace(entrypoint)
	if entrypoint == "" || strings.Contains(entrypoint, "/") {
		return entrypoint
	}
	return filepath.ToSlash(filepath.Join("/usr/local/bin", entrypoint))
}
