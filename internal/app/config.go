package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"g7.mph.tech/mph-tech/autodev/internal/contracts"
	"g7.mph.tech/mph-tech/autodev/internal/configsource"
)

type PathsConfig struct {
	RootDir       string   `json:"root_dir,omitempty"`
	DataDir       string   `json:"data_dir"`
	WorkOrderRepo string   `json:"work_order_repo"`
	RepoRoots     []string `json:"repo_roots,omitempty"`
	SmokeSecrets  string   `json:"smoke_secrets,omitempty"`
}

type GitLabConfig struct {
	BaseURL       string `json:"base_url,omitempty"`
	IssuesProject string `json:"issues_project,omitempty"`
	Token         string `json:"token,omitempty"`
	TokenName     string `json:"token_name,omitempty"`
}

type StoresConfig struct {
	LocksPostgresDSN   string `json:"locks_postgres_dsn,omitempty"`
	RatchetPostgresDSN string `json:"ratchet_postgres_dsn,omitempty"`
	SignalsPostgresDSN string `json:"signals_postgres_dsn,omitempty"`
	CatalogPostgresDSN string `json:"catalog_postgres_dsn,omitempty"`
}

type SecretsConfig struct {
	GCPProject       string `json:"gcp_project,omitempty"`
	LocalKeychainSvc string `json:"local_keychain_service,omitempty"`
}

type ExecutionConfig struct{}

type Env struct {
	ConfigPath          string          `json:"-"`
	Paths               PathsConfig     `json:"paths"`
	GitLab              GitLabConfig    `json:"gitlab"`
	Stores              StoresConfig    `json:"stores"`
	Secrets             SecretsConfig   `json:"secrets"`
	Execution           ExecutionConfig `json:"execution,omitempty"`
	RootDir             string          `json:"-"`
	DataDir             string          `json:"-"`
	StateDir            string          `json:"-"`
	ArtifactDir         string          `json:"-"`
	GitLabDir           string          `json:"-"`
	WorkOrderRepo       string          `json:"-"`
	GitLabBaseURL       string          `json:"-"`
	GitLabIssuesProject string          `json:"-"`
	GitLabToken         string          `json:"-"`
	GitLabTokenName     string          `json:"-"`
	LocksPostgresDSN    string          `json:"-"`
	RatchetPostgresDSN  string          `json:"-"`
	SignalsPostgresDSN  string          `json:"-"`
	CatalogPostgresDSN  string          `json:"-"`
	GCPProject          string          `json:"-"`
	LocalKeychainSvc    string          `json:"-"`
	SmokeSecretsPath    string          `json:"-"`
	RepoRoots           []string        `json:"-"`
}

func Load(path string) (Env, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return Env{}, fmt.Errorf("resolve config path: %w", err)
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return Env{}, fmt.Errorf("read config %s: %w", absPath, err)
	}
	if err := contracts.Validate(contracts.ConfigSchema, absPath, data); err != nil {
		return Env{}, err
	}
	var cfg Env
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Env{}, fmt.Errorf("decode config %s: %w", absPath, err)
	}
	cfg.ConfigPath = absPath
	if err := cfg.validate(); err != nil {
		return Env{}, err
	}
	cfg.prepareResolved()
	if err := cfg.validateContractFiles(); err != nil {
		return Env{}, err
	}
	return cfg, nil
}

func (e *Env) prepareResolved() {
	baseDir := filepath.Dir(e.ConfigPath)
	e.RootDir = resolve(baseDir, e.Paths.RootDir)
	e.DataDir = resolve(baseDir, e.Paths.DataDir)
	e.StateDir = filepath.Join(e.DataDir, "state")
	e.ArtifactDir = filepath.Join(e.DataDir, "artifacts")
	e.GitLabDir = filepath.Join(e.DataDir, "gitlab")
	e.WorkOrderRepo = resolve(baseDir, e.Paths.WorkOrderRepo)
	e.GitLabBaseURL = e.GitLab.BaseURL
	e.GitLabIssuesProject = e.GitLab.IssuesProject
	e.GitLabToken = e.GitLab.Token
	e.GitLabTokenName = e.GitLab.TokenName
	e.LocksPostgresDSN = e.Stores.LocksPostgresDSN
	e.RatchetPostgresDSN = e.Stores.RatchetPostgresDSN
	e.SignalsPostgresDSN = e.Stores.SignalsPostgresDSN
	e.CatalogPostgresDSN = e.Stores.CatalogPostgresDSN
	e.GCPProject = e.Secrets.GCPProject
	e.LocalKeychainSvc = e.Secrets.LocalKeychainSvc
	if e.Paths.SmokeSecrets != "" {
		e.SmokeSecretsPath = resolve(baseDir, e.Paths.SmokeSecrets)
	}
	e.RepoRoots = make([]string, 0, len(e.Paths.RepoRoots))
	for _, root := range e.Paths.RepoRoots {
		e.RepoRoots = append(e.RepoRoots, resolve(baseDir, root))
	}
}

func (e Env) validate() error {
	if e.Paths.RootDir == "" {
		return fmt.Errorf("config %s must set paths.root_dir", e.ConfigPath)
	}
	if e.Paths.DataDir == "" {
		return fmt.Errorf("config %s must set paths.data_dir", e.ConfigPath)
	}
	if e.Paths.WorkOrderRepo == "" {
		return fmt.Errorf("config %s must set paths.work_order_repo", e.ConfigPath)
	}
	if e.GitLab.TokenName == "" {
		return fmt.Errorf("config %s must set gitlab.token_name", e.ConfigPath)
	}
	if e.Secrets.LocalKeychainSvc == "" {
		return fmt.Errorf("config %s must set secrets.local_keychain_service", e.ConfigPath)
	}
	return nil
}

func (e Env) validateContractFiles() error {
	if _, err := configsource.LoadPipelineCatalog(); err != nil {
		return err
	}
	if _, err := configsource.LoadStageSpecs(); err != nil {
		return err
	}
	return nil
}

func resolve(baseDir, value string) string {
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return filepath.Clean(filepath.Join(baseDir, value))
}
