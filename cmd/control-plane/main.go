package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"g7.mph.tech/mph-tech/autodev/internal/app"
	"g7.mph.tech/mph-tech/autodev/internal/configsource"
	"g7.mph.tech/mph-tech/autodev/internal/controlplane"
	"g7.mph.tech/mph-tech/autodev/internal/gitlab"
	"g7.mph.tech/mph-tech/autodev/internal/httpapi"
	"g7.mph.tech/mph-tech/autodev/internal/locks"
	"g7.mph.tech/mph-tech/autodev/internal/ratchet"
	"g7.mph.tech/mph-tech/autodev/internal/runner"
	"g7.mph.tech/mph-tech/autodev/internal/secrets"
	"g7.mph.tech/mph-tech/autodev/internal/signals"
	"g7.mph.tech/mph-tech/autodev/internal/stagecontainer"
	"g7.mph.tech/mph-tech/autodev/internal/store"
)

func main() {
	configPath, args := extractConfigPath(os.Args[1:])
	if len(args) < 1 {
		usage()
	}

	env := mustLoadConfig(configPath)
	specs, err := configsource.LoadStageSpecs()
	if err != nil {
		log.Fatal(err)
	}
	locker := newLocker(env)
	service := controlplane.New(
		store.New(env.StateDir),
		newGitLabAdapter(env),
		locker,
		signaler(env),
		specs,
		env.WorkOrderRepo,
	)

	switch args[0] {
	case "enqueue":
		must(service.EnqueueFromGitLab())
	case "reconcile":
		must(service.Reconcile())
	case "recover-stuck-runs":
		must(service.RecoverStuckRuns(30 * time.Second))
	case "snapshot":
		state, err := service.Snapshot()
		must(err)
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		must(enc.Encode(state))
	case "serve":
		fs := flag.NewFlagSet("serve", flag.ExitOnError)
		addr := fs.String("addr", ":9090", "listen address")
		must(fs.Parse(args[1:]))
		server := httpapi.New(service, httpapi.RuntimeConfig{
			Env:              env,
			RootDir:          env.RootDir,
			DataDir:          env.DataDir,
			GitLabDir:        env.GitLabDir,
			WorkOrderRepo:    env.WorkOrderRepo,
			Specs:            specs,
			PipelineCatalog:  mustLoadPipelineCatalog(),
			LocalIssueImport: env.GitLabBaseURL == "" || env.GitLabIssuesProject == "",
		}, runner.NewResolver(
            runner.NewGitLedger(env.WorkOrderRepo),
            runner.NewPostgresCatalog(mustCatalogDB(env), &stagecontainer.Docker{}),
            0.9,
        ))
		log.Printf("control-plane listening on %s", *addr)
		must(http.ListenAndServe(*addr, server.Handler()))
	case "ratchet-init":
		rs := mustRatchetService(env)
		must(rs.Init(context.Background()))
	case "ratchet-ingest":
		runRatchetIngest(env, args[1:])
	case "ratchet-top":
		runRatchetTop(env, args[1:])
	case "ratchet-activate":
		runRatchetActivate(env, args[1:])
	case "signal-init":
		must(mustSignalService(env).Init(context.Background()))
	case "signal-list":
		runSignalList(env, args[1:])
	default:
		usage()
	}
}

func mustCatalogDB(env app.Env) *pgxpool.Pool {
	if env.CatalogPostgresDSN == "" {
		log.Fatal("stores.catalog_postgres_dsn must be set for Postgres catalog")
	}
	pool, err := pgxpool.New(context.Background(), env.CatalogPostgresDSN)
	if err != nil {
		log.Fatal(err)
	}
	return pool
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: control-plane [--config path] <enqueue|reconcile|recover-stuck-runs|snapshot|serve|ratchet-init|ratchet-ingest|ratchet-top|ratchet-activate|signal-init|signal-list>")
	os.Exit(2)
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

func newLocker(env app.Env) locks.Manager {
	if env.LocksPostgresDSN == "" {
		return locks.NoopManager{}
	}
	locker, err := locks.NewPostgresManager(context.Background(), env.LocksPostgresDSN)
	if err != nil {
		log.Fatal(err)
	}
	return locker
}

func mustRatchetService(env app.Env) *ratchet.Service {
	if env.RatchetPostgresDSN == "" {
		log.Fatal("stores.ratchet_postgres_dsn must be set in the active config file for ratchet commands")
	}
	store, err := ratchet.NewPostgresStore(context.Background(), env.RatchetPostgresDSN)
	if err != nil {
		log.Fatal(err)
	}
	return ratchet.NewService(store)
}

func runRatchetIngest(env app.Env, args []string) {
	fs := flag.NewFlagSet("ratchet-ingest", flag.ExitOnError)
	filePath := fs.String("file", "", "path to finding event json")
	must(fs.Parse(args))
	if *filePath == "" {
		log.Fatal("ratchet-ingest requires --file")
	}

	payload, err := os.ReadFile(*filePath)
	must(err)

	var event ratchet.FindingEvent
	must(json.Unmarshal(payload, &event))
	result, err := mustRatchetService(env).IngestFinding(context.Background(), event)
	must(err)
	writePrettyJSON(result)
}

func runRatchetTop(env app.Env, args []string) {
	fs := flag.NewFlagSet("ratchet-top", flag.ExitOnError)
	stage := fs.String("stage", "", "stage name")
	repoScope := fs.String("repo-scope", "", "repo scope")
	environment := fs.String("environment", "", "environment scope")
	serviceScope := fs.String("service-scope", "", "service scope")
	limit := fs.Int("limit", 5, "maximum invariants to return")
	must(fs.Parse(args))
	if *stage == "" {
		log.Fatal("ratchet-top requires --stage")
	}

	ranked, err := mustRatchetService(env).RankedInvariants(context.Background(), ratchet.RetrievalRequest{
		Stage:        *stage,
		RepoScope:    *repoScope,
		Environment:  *environment,
		ServiceScope: *serviceScope,
		Limit:        *limit,
	})
	must(err)
	writePrettyJSON(ranked)
}

func runRatchetActivate(env app.Env, args []string) {
	fs := flag.NewFlagSet("ratchet-activate", flag.ExitOnError)
	proposalID := fs.String("proposal-id", "", "proposal id")
	mode := fs.String("enforcement-mode", "warn", "enforcement mode")
	must(fs.Parse(args))
	if *proposalID == "" {
		log.Fatal("ratchet-activate requires --proposal-id")
	}
	id, err := strconv.ParseInt(*proposalID, 10, 64)
	must(err)

	invariant, err := mustRatchetService(env).ActivateProposal(context.Background(), id, *mode)
	must(err)
	writePrettyJSON(invariant)
}

func writePrettyJSON(value any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	must(enc.Encode(value))
}

func signaler(env app.Env) signals.Emitter {
	if env.SignalsPostgresDSN == "" {
		return signals.NewService(signals.NoopStore{})
	}
	return mustSignalService(env)
}

func mustSignalService(env app.Env) *signals.Service {
	if env.SignalsPostgresDSN == "" {
		log.Fatal("stores.signals_postgres_dsn must be set in the active config file for signal commands")
	}
	store, err := signals.NewPostgresStore(context.Background(), env.SignalsPostgresDSN)
	if err != nil {
		log.Fatal(err)
	}
	return signals.NewService(store)
}

func runSignalList(env app.Env, args []string) {
	fs := flag.NewFlagSet("signal-list", flag.ExitOnError)
	repoScope := fs.String("repo-scope", "", "repo scope")
	stage := fs.String("stage", "", "stage")
	status := fs.String("status", "open", "signal status")
	limit := fs.Int("limit", 20, "signal list limit")
	must(fs.Parse(args))

	result, err := mustSignalService(env).ListSignals(context.Background(), signals.ListRequest{
		RepoScope: *repoScope,
		Stage:     *stage,
		Status:    *status,
		Limit:     *limit,
	})
	must(err)
	writePrettyJSON(result)
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

func mustLoadConfig(path string) app.Env {
	env, err := app.Load(path)
	if err != nil {
		log.Fatal(err)
	}
	return env
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
