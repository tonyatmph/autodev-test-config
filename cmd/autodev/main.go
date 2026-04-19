package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"g7.mph.tech/mph-tech/autodev/internal/runner"
	"g7.mph.tech/mph-tech/autodev/internal/stagecontainer"
)

func main() {
	goalName := flag.String("goal", "", "The goal to resolve (e.g., 'package')")
	repoRoot := flag.String("repo", ".", "The repository root")
	dbURL := flag.String("db", os.Getenv("DATABASE_URL"), "Postgres database URL")
	fitness := flag.Float64("fitness", 0.9, "Fitness threshold")
	flag.Parse()

	if *goalName == "" {
		fmt.Println("Usage: autodev resolve --goal <name> [--repo <path>]")
		os.Exit(1)
	}

	// 1. Initialize Trinity Primitives
	ledger := runner.NewGitLedger(*repoRoot)
	
	var catalog runner.Catalog
	dockerRunner := &stagecontainer.Docker{}
	if *dbURL != "" {
		pool, err := pgxpool.New(context.Background(), *dbURL)
		if err != nil {
			log.Fatalf("failed to connect to db: %v", err)
		}
		catalog = runner.NewPostgresCatalog(pool, dockerRunner)
	} else {
		// Fallback for demo: use a mock catalog if no DB URL provided
		catalog = runner.NewDBStageCatalog(nil, dockerRunner)
	}

	// 2. Initialize Kernel
	kernel := &runner.Resolver{
		Ledger:           ledger,
		Catalog:          catalog,
		FitnessThreshold: *fitness,
	}

	// 3. Resolve Intent
	log.Printf("Resolving goal: %s", *goalName)
	ctx := context.Background()
	finalSHA, err := kernel.Resolve(ctx, runner.Goal{
		Contract: runner.Contract{Name: *goalName, Version: "1.0"},
		InputSHA: "SHA-INITIAL",
	})
	
	if err != nil {
		log.Fatalf("Resolution failed: %v", err)
	}

	log.Printf("Resolution successful: %s", finalSHA)
}
