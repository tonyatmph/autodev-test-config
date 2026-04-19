package isolation

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"g7.mph.tech/mph-tech/autodev/internal/model"
	repoplane "g7.mph.tech/mph-tech/autodev/internal/repos"
)

type Scope struct {
	Root     string
	Patterns []string
	AllowAll bool
	Reason   string
}

type Sandbox struct {
	Stage            string
	Environment      string
	RunAs            model.ExecutionIdentity
	WriteAs          model.ExecutionIdentity
	Permissions      model.StagePermissions
	RepoRoots        []string
	WorkspaceRoot    string
	WritableScopes   []Scope
	repoControlRoots map[string]string
}

func New(rootDir, dataDir string, repoRoots []string, spec model.StageSpec, run model.RunRequest) *Sandbox {
	container := spec.ContainerConfig()
	sandbox := &Sandbox{
		Stage:            spec.Name,
		Environment:      spec.Environment(),
		RunAs:            container.RunAs,
		WriteAs:          container.WriteAs,
		Permissions:      container.Permissions,
		RepoRoots:        append([]string(nil), repoRoots...),
		WorkspaceRoot:    filepath.Join(dataDir, "workspaces", run.ID, spec.Name),
		repoControlRoots: map[string]string{},
	}
	sandbox.allowAll(sandbox.WorkspaceRoot, "workspace")
	sandbox.addIdentityScopes(rootDir, dataDir, run)
	return sandbox
}

func (s *Sandbox) Summary() map[string]any {
	scopes := make([]map[string]any, 0, len(s.WritableScopes))
	for _, scope := range s.WritableScopes {
		scopes = append(scopes, map[string]any{
			"root":      scope.Root,
			"patterns":  append([]string(nil), scope.Patterns...),
			"allow_all": scope.AllowAll,
			"reason":    scope.Reason,
		})
	}
	repoControl := make([]map[string]string, 0, len(s.repoControlRoots))
	for root, reason := range s.repoControlRoots {
		repoControl = append(repoControl, map[string]string{
			"root":   root,
			"reason": reason,
		})
	}
	return map[string]any{
		"stage":        s.Stage,
		"environment":  s.Environment,
		"run_as":       s.RunAs,
		"write_as":     s.WriteAs,
		"permissions":  s.Permissions,
		"workspace":    s.WorkspaceRoot,
		"writable":     scopes,
		"repo_control": repoControl,
	}
}

func (s *Sandbox) MkdirAll(path string, perm os.FileMode) error {
	if err := s.RequireWrite(path); err != nil {
		return err
	}
	return os.MkdirAll(path, perm)
}

func (s *Sandbox) WriteFile(path string, data []byte, perm os.FileMode) error {
	if err := s.RequireWrite(path); err != nil {
		return err
	}
	parent := filepath.Dir(path)
	if parent != "" && parent != "." {
		if err := s.RequireWrite(parent); err != nil {
			return err
		}
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, perm)
}

func (s *Sandbox) RequireWrite(path string) error {
	if s.WriteAs == "" {
		return nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve write path %q: %w", path, err)
	}
	for _, scope := range s.WritableScopes {
		if scope.allows(abs) {
			return nil
		}
	}
	return fmt.Errorf("runtime sandbox denies %s write to %s for write_as %q", s.Stage, abs, s.WriteAs)
}

func (s *Sandbox) RequireRepoControl(path, operation string) error {
	if s.WriteAs == "" {
		return nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve repo control path %q: %w", path, err)
	}
	for root := range s.repoControlRoots {
		if sameOrUnder(root, abs) {
			return nil
		}
	}
	return fmt.Errorf("runtime sandbox denies %s repo control (%s) at %s for write_as %q", s.Stage, operation, abs, s.WriteAs)
}

func (s *Sandbox) allowAll(root, reason string) {
	root = filepath.Clean(root)
	s.WritableScopes = append(s.WritableScopes, Scope{
		Root:     root,
		AllowAll: true,
		Reason:   reason,
	})
}

func (s *Sandbox) allowPatterns(root string, patterns []string, reason string) {
	cleaned := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(filepath.ToSlash(pattern))
		if pattern == "" {
			continue
		}
		cleaned = append(cleaned, pattern)
	}
	if len(cleaned) == 0 {
		return
	}
	root = filepath.Clean(root)
	s.WritableScopes = append(s.WritableScopes, Scope{
		Root:     root,
		Patterns: cleaned,
		Reason:   reason,
	})
}

func (s *Sandbox) allowRepoControl(root, reason string) {
	root = filepath.Clean(root)
	if root == "" {
		return
	}
	if _, ok := s.repoControlRoots[root]; ok {
		return
	}
	s.repoControlRoots[root] = reason
}

func (s *Sandbox) GrantRepoControl(root, reason string) {
	s.allowRepoControl(root, reason)
}

func (s *Sandbox) GrantWriteAccess(root, reason string) {
	s.allowAll(root, reason)
}

func (s *Sandbox) GrantPatternAccess(root string, patterns []string, reason string) {
	s.allowPatterns(root, patterns, reason)
}

func (s *Sandbox) addIdentityScopes(rootDir, dataDir string, run model.RunRequest) {
	if s.WriteAs == "" || !s.WriteAs.Valid() {
		return
	}
	target := run.DeliveryTarget()
	if s.Permissions.AllowsWritable(model.StageSurfaceComponents) || s.Permissions.AllowsRepoControl(model.StageSurfaceComponents) {
		for _, component := range target.SelectedDeliveryComponents() {
			for _, repo := range component.AllRepositories() {
				rules := repo.OwnershipRulesFor(s.WriteAs)
				if len(repo.Ownership) == 0 {
					rules = component.OwnershipRulesFor(s.WriteAs)
				}
				patterns := mutablePatterns(rules)
				for _, candidate := range repoplane.RepoCandidates(rootDir, repo.Repo.ProjectPath, s.RepoRoots...) {
					if len(patterns) > 0 && s.Permissions.AllowsWritable(model.StageSurfaceComponents) {
						s.allowPatterns(candidate, patterns, fmt.Sprintf("%s component %s", s.WriteAs, component.Name))
					}
					if s.Permissions.AllowsRepoControl(model.StageSurfaceComponents) {
						s.allowRepoControl(candidate, fmt.Sprintf("%s component %s", s.WriteAs, component.Name))
					}
				}
			}
		}
	}
	if s.WriteAs == model.ExecutionIdentityGoverned && (s.Permissions.AllowsWritable(model.StageSurfaceJournal) || s.Permissions.AllowsRepoControl(model.StageSurfaceJournal)) {
		journal := run.DeliveryTarget().JournalOrDefault()
		for _, candidate := range gitOpsCandidates(rootDir, dataDir, journal.Repo.ProjectPath, journal.Repo.MaterializationPath, s.RepoRoots...) {
			if s.Permissions.AllowsWritable(model.StageSurfaceJournal) {
				s.allowAll(candidate, "governed journal repo")
				s.allowPatterns(candidate, []string{
					filepath.ToSlash(strings.Trim(journal.Path, "/")) + "/**",
				}, "governed journal")
			}
			if s.Permissions.AllowsRepoControl(model.StageSurfaceJournal) {
				s.allowRepoControl(candidate, "governed journal")
			}
		}
	}
	if s.WriteAs == model.ExecutionIdentityGoverned && (s.Permissions.AllowsWritable(model.StageSurfaceGitOps) || s.Permissions.AllowsRepoControl(model.StageSurfaceGitOps)) {
		env := strings.TrimSpace(s.Environment)
		targetEnv := run.EnvironmentTarget(env)
		if targetEnv.GitOpsRepo.ProjectPath != "" && targetEnv.GitOpsRepo.Path != "" {
			for _, candidate := range gitOpsCandidates(rootDir, dataDir, targetEnv.GitOpsRepo.ProjectPath, targetEnv.GitOpsRepo.MaterializationPath, s.RepoRoots...) {
				if s.Permissions.AllowsWritable(model.StageSurfaceGitOps) {
					s.allowAll(candidate, fmt.Sprintf("governed gitops repo %s", env))
					s.allowPatterns(candidate, []string{
						filepath.ToSlash(strings.Trim(targetEnv.GitOpsRepo.Path, "/")) + "/**",
					}, fmt.Sprintf("governed gitops %s", env))
				}
				if s.Permissions.AllowsRepoControl(model.StageSurfaceGitOps) {
					s.allowRepoControl(candidate, fmt.Sprintf("governed gitops %s", env))
				}
			}
		}
	}
	if s.WriteAs == model.ExecutionIdentityGenerator && (s.Permissions.AllowsWritable(model.StageSurfaceJournal) || s.Permissions.AllowsRepoControl(model.StageSurfaceJournal)) {
		journal := run.DeliveryTarget().JournalOrDefault()
		candidates := gitOpsCandidates(rootDir, dataDir, journal.Repo.ProjectPath, journal.Repo.MaterializationPath, s.RepoRoots...)
		generatorFallback := filepath.Join(dataDir, "generator", filepath.FromSlash(journal.Repo.ProjectPath))
		candidates = append(candidates, generatorFallback)
		for _, candidate := range uniqueCandidates(candidates) {
			if s.Permissions.AllowsWritable(model.StageSurfaceJournal) {
				s.allowAll(candidate, "generator journal repo")
				s.allowPatterns(candidate, []string{
					filepath.ToSlash(filepath.Join(strings.Trim(journal.Path, "/"), "generated")) + "/**",
				}, "generator durable outputs")
			}
			if s.Permissions.AllowsRepoControl(model.StageSurfaceJournal) {
				s.allowRepoControl(candidate, "generator durable outputs")
			}
		}
	}
}

func gitOpsCandidates(rootDir, _ string, projectPath, materializationPath string, extraRoots ...string) []string {
	candidates := repoplane.RepoCandidates(rootDir, projectPath, extraRoots...)
	if strings.TrimSpace(materializationPath) != "" {
		candidates = append(candidates, filepath.Clean(materializationPath))
	}
	seen := make(map[string]struct{}, len(candidates)+1)
	out := make([]string, 0, len(candidates)+1)
	for _, candidate := range candidates {
		candidate = filepath.Clean(candidate)
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		out = append(out, candidate)
	}
	return out
}

func uniqueCandidates(values []string) []string {
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

func mutablePatterns(rules []model.PathOwnershipRule) []string {
	var patterns []string
	for _, rule := range rules {
		if !rule.Mutable {
			continue
		}
		patterns = append(patterns, rule.Paths...)
	}
	return patterns
}

func (s Scope) allows(path string) bool {
	if !sameOrUnder(s.Root, path) {
		return false
	}
	if s.AllowAll {
		return true
	}
	rel, err := filepath.Rel(s.Root, path)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	for _, pattern := range s.Patterns {
		if matchPattern(pattern, rel) {
			return true
		}
	}
	return false
}

func sameOrUnder(root, path string) bool {
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func matchPattern(pattern, rel string) bool {
	pattern = strings.TrimPrefix(filepath.ToSlash(pattern), "./")
	rel = strings.TrimPrefix(filepath.ToSlash(rel), "./")
	if pattern == "" || pattern == "." {
		return rel == "." || rel == ""
	}
	if pattern == rel {
		return true
	}
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		return rel == prefix || strings.HasPrefix(rel, prefix+"/")
	}
	ok, err := filepath.Match(filepath.FromSlash(pattern), filepath.FromSlash(rel))
	return err == nil && ok
}
