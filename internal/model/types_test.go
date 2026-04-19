package model

import (
	"reflect"
	"testing"
)

func TestOrderedSelectedComponentNamesRespectsDependencies(t *testing.T) {
	target := DeliveryTarget{
		SelectedComponents: []string{"mobile", "prompts", "console", "config", "api"},
		Components: DeliveryComponents{
			"api":     {Name: "api"},
			"config":  {Name: "config", DependsOn: []string{"api"}},
			"console": {Name: "console", DependsOn: []string{"api", "config"}},
			"prompts": {Name: "prompts", DependsOn: []string{"config"}},
			"mobile":  {Name: "mobile", DependsOn: []string{"api", "config", "prompts"}},
		},
	}

	got := target.OrderedSelectedComponentNames()
	want := []string{"api", "config", "console", "prompts", "mobile"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected ordered components: got %v want %v", got, want)
	}
}

func TestCanonicalWorkOrderUsesExplicitWorkOrderOnly(t *testing.T) {
	issue := DeliveryIssue{
		ID: "issue-1",
		WorkOrder: WorkOrder{
			ID:               "wo-issue-1",
			SourceIssueID:    "issue-1",
			RequestedOutcome: "ship selected components",
			Delivery: DeliveryTarget{
				PrimaryComponent:   "api",
				SelectedComponents: []string{"api", "console"},
				Components: DeliveryComponents{
					"api": {
						Name: "api",
						Repo: RepoTarget{ProjectPath: "mph-tech/example-api"},
						Release: ReleaseDefinition{
							Application: ApplicationRelease{
								ArtifactName: "example-api",
								ImageRepo:    "registry.mph.tech/example-api",
							},
						},
					},
				},
			},
		},
	}

	order := issue.CanonicalWorkOrder()
	if order.ID != "wo-issue-1" || order.SourceIssueID != "issue-1" {
		t.Fatalf("expected explicit work order to be preserved, got %+v", order)
	}
	if order.Delivery.ApplicationRepo.ProjectPath != "" {
		t.Fatalf("expected no repo fallback, got %q", order.Delivery.ApplicationRepo.ProjectPath)
	}
	if order.Delivery.Release.Application.ImageRepo != "" {
		t.Fatalf("expected no release fallback, got %q", order.Delivery.Release.Application.ImageRepo)
	}
}

func TestDocumentationStatusRequiresSelectedDocsComponent(t *testing.T) {
	target := DeliveryTarget{
		SelectedComponents: []string{"api"},
		Documentation: DocumentationPolicy{
			Required:      true,
			DocsComponent: "docs",
		},
		Components: DeliveryComponents{
			"api":  {Name: "api"},
			"docs": {Name: "docs"},
		},
	}

	required, component, satisfied, reason := target.DocumentationStatus()
	if !required {
		t.Fatal("expected documentation to be required")
	}
	if component != "docs" {
		t.Fatalf("expected docs component, got %q", component)
	}
	if satisfied {
		t.Fatal("expected documentation policy to be unsatisfied")
	}
	if reason == "" {
		t.Fatal("expected documentation failure reason")
	}
}

func TestDeliveryComponentOwnershipRulesForIdentity(t *testing.T) {
	component := DeliveryComponent{
		Name: "api",
		Ownership: []PathOwnershipRule{
			{Identity: ExecutionIdentityAgent, Paths: []string{"contracts/**"}, Mutable: true},
		},
		Repositories: []ComponentRepository{
			{
				Repo: RepoTarget{ProjectPath: "mph-tech/api"},
				Ownership: []PathOwnershipRule{
					{Identity: ExecutionIdentityGenerator, Paths: []string{"generated/**"}, Mutable: true},
				},
			},
		},
	}

	rules := component.OwnershipRulesFor(ExecutionIdentityGenerator)
	if len(rules) != 1 {
		t.Fatalf("expected generator ownership rule to be present, got %d", len(rules))
	}
	if rules[0].Identity != ExecutionIdentityGenerator {
		t.Fatalf("expected generator identity, got %s", rules[0].Identity)
	}
}

func TestDeliveryTargetHasOwnershipRules(t *testing.T) {
	target := DeliveryTarget{
		SelectedComponents: []string{"api"},
		Components: DeliveryComponents{
			"api": {
				Name: "api",
				Repositories: []ComponentRepository{
					{
						Repo: RepoTarget{ProjectPath: "mph-tech/api"},
						Ownership: []PathOwnershipRule{
							{Identity: ExecutionIdentityGoverned, Paths: []string{"prompts/**"}, Mutable: false},
						},
					},
				},
			},
		},
	}

	if !target.HasOwnershipRules(ExecutionIdentityGoverned) {
		t.Fatal("expected governed identity rules to be detected")
	}
	if target.HasOwnershipRules(ExecutionIdentityGenerator) {
		t.Fatal("did not expect generator rules for this target")
	}
}

func TestJournalOrDefaultReturnsDeclaredJournalOnly(t *testing.T) {
	target := DeliveryTarget{
		Journal: JournalTarget{
			Name:     "journal",
			Path:     "runs",
			Strategy: "git",
			Repo: RepoTarget{
				ProjectPath:         "work-orders",
				DefaultBranch:       "main",
				WorkingBranchPrefix: "autodev",
			},
		},
	}
	journal := target.JournalOrDefault()
	if journal.Name != "journal" || journal.Path != "runs" || journal.Strategy != "git" {
		t.Fatalf("expected declared journal to be preserved, got %+v", journal)
	}
	if journal.Repo.ProjectPath != "work-orders" {
		t.Fatalf("expected declared journal repo, got %q", journal.Repo.ProjectPath)
	}
}
