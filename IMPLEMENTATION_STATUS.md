# Implementation Status

This document is the working checklist for Autodev.

Status meanings:

- `Done`: implemented and working in the current scaffold
- `First Pass`: implemented as an initial/scaffolded or partially real version
- `Remaining`: not yet implemented or not yet production-grade

## Overall Summary

### Done

- issue-centric control-plane lifecycle
- typed stage contract and shared runner
- explicit contract boundary: config defines behavior, Go propagates and
  observes it, runtime/container own enforcement and in-container execution
- explicit architectural decision that policy constrains pipeline
  materialization and every stage, not just top-level approval
- explicit multi-plane split for control, execution, release, repo, policy, memory, and agent boundaries
- explicit architectural decision that durable state is Git-backed and operational state is Postgres-backed
- universal run-scoped Git materialization in the shared stage primitive
- explicit pinned-ref support for deterministic repo materialization in test and e2e lanes
- universal stage container primitive with stage-configured run/write identities, permissions, runtime user hints, and materialized surfaces
- rollback-aware multi-environment DAG
- ratchet, stats, cost, timing, and signal subsystems
- Postgres-backed locks, ratchets, and signals
- enforced stage secret resolution through local keychain and GCP Secret Manager
- explicit issue -> work-order separation in the domain model
- multi-component delivery objects with dependency ordering, docs-as-a-surface support, and per-component source stamps
- Git-backed work-order journal commits, generator commits, and governed GitOps promotion commits recorded in runtime metadata
- full stage reports and run indexes committed into the work-order repo for every completed stage
- `review` and `release_prepare` now consume prior stage evidence from the work-order journal first
- first-pass reusable stage image build lane with a Git-backed runtime-substrate commit anchor
- built-in operator dashboard served by `control-plane serve` for issue import, run materialization, realtime status, and journal inspection

### First Pass

- self-hosted GitLab API integration
- GitOps-shaped promotion and rollback stages
- bounded agent-driver interface
- stage bodies
- shared Dockerfile tagged per-stage rather than fully specialized stage image families
- cost attribution
- pipeline signal synthesis
- durable stage reports are Git-backed, but raw logs and bulky artifacts still primarily live outside the work-order repo

### Remaining

- real issue translator from human issue text to work order
- policy-constrained pipeline intent -> execution-plan materialization
- separate governed config repo baked into the platform build as the long-term
  trust anchor
- pipeline branches derived from config-repo `main` with parallel legacy vs
  upgraded validation during rebases
- explicit policy hierarchy implementation across global, pipeline-family,
  environment, repo/component, and stage scopes
- real security/test/review depth
- real Argo CD observation and production promotion path
- production-grade state store and distributed scheduling

## Minimum Dev Run Plan

The delivery path should be proven in two stages.

### Stage 1: Local-Only Dev Validation

Goal:

- use a real GitLab issue
- use a real codebase and real local Git checkouts
- keep the pipeline, generated outputs, and GitOps changes local
- avoid real cluster deployment or real infra mutation
- iterate quickly until the delivery spine is stable

Success criteria:

- [ ] real GitLab issue is ingested through the self-hosted GitLab adapter
- [x] one real app repo is resolved locally under `paths.repo_roots`
- [x] `implement` creates a real branch and commit against a run-scoped local Git materialization
- [x] `implement` emits `mr_proposal`
- [x] `generate` commits durable generated outputs and records `generator_commit`
- [x] `security` emits real evidence from the real repo contents
- [x] `test` executes at least one real repo-defined command and emits evidence
- [x] `review` consumes real evidence and docs policy
- [x] `release_prepare` writes and commits a real journal entry
- [x] `promote_local` and `promote_dev` write and commit real GitOps changes into the run-scoped GitOps materialization
- [x] run metadata includes `work_order_commit`, `journal_history`, `generator_commits`, and `promotion_commits`
- [x] the lane stops cleanly at `awaiting_approval` without requiring a real cluster

Not required in Stage 1:

- real remote MR creation
- real GitOps MR creation/merge
- real Argo CD observation
- real Terraform apply
- real DB migration execution

### Stage 2: Real Dev Promotion

Goal:

- take the already-proven local delivery spine
- connect it to real remote GitLab and a real GitOps repo
- let one change reach the real `dev` environment

Success criteria:

- [x] `implement` creates or updates a real GitLab MR through the configured adapter
- [x] `promote_dev` creates or updates a real GitOps MR through the configured adapter
- [ ] the GitOps change lands in the real dev branch by human merge or controlled platform merge
- [ ] `observe_dev` reads real Argo CD or cluster rollout state
- [ ] the real dev run can be traced through issue id, run id, app commit, generator commit, release journal commit, and GitOps commit

Minimum critical path:

1. Stage 1 must be green and repeatable.
2. Then wire remote app-repo MR creation.
3. Then wire remote GitOps MR creation.
4. Then wire real `observe_dev`.
5. Then run one real issue end to end.

## Architecture Hygiene

### Done

- [x] canonical work-order helpers centralized in the model
- [x] repo/environment/service accessors centralized on `RunRequest`
- [x] stage policy moved into a dedicated `internal/policy` package
- [x] synthetic stage behavior split out from the shared runner lifecycle path
- [x] stage dispatch moved behind a handler registry
- [x] release manifest and release bundle ownership moved behind the universal container contract
- [x] repo discovery and source stamping moved into `internal/repos`
- [x] bounded agent-driver interface added in `internal/agent`
- [x] primitive ownership and lifecycle guidance documented

### Remaining

- [ ] remove long-term reliance on legacy top-level target fields
- [ ] promote more real stage handlers/adapters to replace synthetic behavior
- [ ] separate stage packages from the shared runner when handler count grows further
- [ ] make policy a first-class materialization input, not just a runtime gate

## Policy-Driven Materialization

### Done

- [x] policy plane exists as an explicit architectural boundary
- [x] stage execution already carries policy evidence and machine-enforced success contracts

### First Pass

- [x] stage-level policy gating
- [x] documentation and approval checks attached to stage execution
- [x] `policy_evaluation` exposes `pipeline_scope` plus a `stage_scope` map so
  each stage can read the exact contract that constrained it, and the control
  plane always prefers the Git-backed plan artifacts from the work-order journal
  before falling back to DB metadata

### Remaining

- [ ] `Issue -> WorkOrder -> PipelineIntent -> PolicyEvaluation -> PipelineBuildPlan -> PipelineExecutionPlan -> RunJournal`
- [ ] hierarchical policy model:
  - global
  - pipeline-family
  - environment
  - repo/component
  - stage
- [ ] make policy shape the executable plan before stage queueing
- [ ] persist policy decisions as durable plan evidence in the work-order repo
- [ ] make pipeline-family selection actually shape the execution graph
- [ ] add first-class parallel groups, gather stages, and bounded loop semantics
- [ ] add first-class candidate strategy / parallel pipeline evaluation
- [ ] add issue-family fitness scoring and durable adjudication reports

- [x] `plan` emits first-class `pipeline_intent`, `policy_evaluation`,
  `pipeline_build_plan`, and `pipeline_execution_plan` artifacts
- [x] issue type is now a first-class work-order contract and feeds pipeline
  family selection during plan materialization
- [x] checked-in pipeline catalog declares accepted issue types, optimization
  goals, testing policy, and new-pipeline creation semantics
- [x] immutable testing / inspection policy is now carried in
  `pipeline_intent` and `pipeline_execution_plan`
- [x] issue-family testing policy is now explicit enough to represent
  test-plan-first development lanes and unreadable-but-executable test surfaces
- [x] the active stage catalog and image catalog are propagated into the
  universal container as contract inputs
- [x] the meta-pipeline self-host validation lane now runs inside
  `build-stage-images` so `make meta-validate` and the CI job rebuild the stage
  images before invoking `stage-runner`, ensuring the catalog that drives the
  materialized plan is the catalog that produced the images in the same loop
  and the resulting pipeline artifacts are journaled in the work-order repo

## Core Control Plane

### Done

- [x] GitLab-issue-centric run model
- [x] run creation from issue intake
- [x] DAG stage queueing
- [x] worker claim / heartbeat / complete lifecycle
- [x] retry and stale-attempt recovery
- [x] approval gating for production promotion
- [x] rollback stage queueing after failed observation
- [x] issue comment and label mirroring
- [x] run/attempt persistence model
- [x] journal metadata from stage outputs threaded into run/attempt metadata (`journal_entry`, `journal_history`, `last_journal`)
- [x] Git-backed work-order journal commit per run stored in `work_order_commit`
- [x] generator commit metadata persisted on attempts/runs (`generator_commit`, `generator_commits`)
- [x] promotion GitOps commit metadata persisted on attempts/runs (`promotion_gitops_commit`, `promotion_commits`)

### First Pass

- [x] JSON-file-backed persisted state

### Remaining

- [ ] Postgres-backed control-plane state
- [ ] distributed queue / event bus
- [ ] fairness scheduling across repos / tenants
- [ ] HA control-plane deployment
- [ ] stronger recovery semantics across process restarts

## Delivery Issue and Work Order Model

### Done

- [x] separate `DeliveryIssue` and `WorkOrder` types
- [x] canonical work-order normalization
- [x] compatibility fallback from legacy top-level target fields
- [x] translation metadata on work orders

### First Pass

- [x] fenced JSON work order embedded in GitLab issue descriptions

### Remaining

- [ ] human-friendly issue template
- [ ] issue -> work-order translator
- [ ] translation validation and failure feedback loop
- [ ] issue-authoring UX that does not require raw machine JSON
- [ ] explicit experiment-set model for running the same issue through multiple
  candidate pipelines or implementation strategies

## Multi-Component Delivery

### Done

- [x] generic component map instead of fixed `api/console/...` struct
- [x] selected component combinations
- [x] primary component detection
- [x] component dependency ordering with `depends_on`
- [x] component-aware release manifest
- [x] per-component source stamp fields
- [x] stage output includes ordered selected components
- [x] docs as a standard component surface
- [x] first-pass documentation policy gate

### First Pass

- [x] real local Git-derived source stamping with synthetic fallback
- [x] implement emits machine-readable MR proposal payloads

### Remaining

- [ ] component-aware repo write workflows
- [ ] component-aware promotion policies
- [ ] component-specific health and rollout observation
- [ ] real documentation diff validation and freshness checks
- [ ] explicit repo topology for authored, generated, governed, journal, and GitOps surfaces
- [ ] explicit wiki documentation surface for architecture docs,
  implementation notes, roadmaps, and related knowledge outputs

## Test-Plan And Adjudication Model

### Done

- [x] immutable testing / inspection policy is now part of the work order and
  materialized plan
- [x] issue families now carry optimization goals that later scoring can use

### Remaining

- [ ] first-class `TestPlan` artifact
- [ ] test-plan builder stage
- [ ] test materialization stage that produces immutable hidden/executable test
  surfaces
- [ ] scatter-gather inspection execution
- [ ] gather stage that synthesizes parallel findings into one remediation model
- [ ] adjudication report for choosing the best admissible candidate by
  issue-family fitness

## Stage Contract / Universal Primitive

### Done

- [x] typed `StageSpec`
- [x] stage dependency model
- [x] retry / timeout / concurrency declarations
- [x] artifact policy declarations
- [x] allowed secret declarations
- [x] approval-required stage declarations
- [x] shared stage runner runtime
- [x] isolated workspace preparation
- [x] prompt/tooling repo injection
- [x] typed outputs and artifacts
- [x] stage stats and cost surface
- [x] stage sub-stage timing surface
- [x] stage-configured universal container primitive
- [x] real container execution path for worker/local lanes
- [x] explicit `run_as` and `write_as` fields in stage config
- [x] first-class stage permissions object
- [x] explicit materialized surfaces in stage config

### First Pass

- [x] shared runner lifecycle plus explicit handler registry
- [x] bounded agent interface present but not yet used by real model-backed handlers

### Remaining

- [ ] real implementations for the stage bodies
- [ ] stricter schema validation on stage inputs/outputs
- [ ] tighter runtime enforcement of stage capabilities
- [ ] fully specialized per-stage image families beyond the shared runtime substrate

## Stage Image Build Pipeline

### Done

- [x] Git-backed stage image catalog surface
- [x] removed `execution.image_catalog`; runtime trust now anchors on `containers/runtime-substrate.commit`
- [x] container runtime resolves stage images from the catalog
- [x] reusable build script for building the stage image set
- [x] CI job that builds the image set and publishes the catalog as an artifact

### First Pass

- [x] one shared Dockerfile builds the universal runtime substrate
- [x] explicit descendant stage images inherit from that substrate

### Remaining

- [ ] dedicated container repo or clearly separated container surface
- [ ] fully specialized stage image profiles and Dockerfiles
- [ ] immutable digest pinning back into stage specs or materialized execution plans
- [ ] policy-triggered rebuilds of impacted stage images
- [ ] self-hosted lane that builds stage images and then consumes the newly built catalog in the same validation flow

## GitLab Integration

### Done

- [x] issue and label model
- [x] file-backed local adapter
- [x] real adapter boundary

### First Pass

- [x] self-hosted GitLab API listing of issues
- [x] issue comment creation via notes API
- [x] issue label updates via issues API
- [x] keychain-backed GitLab token resolution
- [x] typed implement and promotion MR proposal surfaces for future GitLab MR creation
- [x] typed GitLab MR create-request builders for implement and promotion payloads

### Remaining

- [x] merge request creation/update through the GitLab adapter
- [ ] reviewer and approval integration
- [ ] better issue filtering and pagination behavior
- [ ] stronger GitLab auth / OIDC model

## Identity Governance

### Done

- [x] documented the agent/generator/governed identities and their writable surfaces
- [x] noted governed surfaces (prompts, stage specs, policies) in the operator guide

### First Pass

- [x] local sample issue flags the governed surfaces it relies on
- [x] in-process runtime sandbox enforces identity-aware writable paths and repo-control roots for identity-bound stages

### Remaining

- [x] container-user execution path in the runner image
- [ ] host-user execution path per identity
- [x] stage execution no longer supports process mode; all real stage work runs through the container runner image
- [ ] path-level audit trails linking edits to agent/generator/governed identities
- [ ] distinct credentials and checkout ownership per identity

## Durability And Audit Model

### Done

- [x] documented Git as the durable audit surface
- [x] documented Postgres as the operational audit surface
- [x] documented that operational records should index Git SHAs and related release identities
- [x] Git-backed work-order journal
- [x] Git-backed release journal via `release_prepare`
- [x] Git-backed generator durability for `generate`
- [x] Git-backed local GitOps promotion commits for `promote_local` / `promote_dev`

### First Pass

- [x] operational DBs already track runtime state for locks, ratchets, and signals

### Remaining

- [ ] DB-level correlation from run/attempt ids to durable Git commits across authored, generated, governed, and GitOps repos
- [ ] remote Git-backed authored/generated/governed/journal surfaces instead of only local e2e-fixture-backed ones

## GitOps Promotion / Rollback

### Done

- [x] multi-environment model: `local -> dev -> prod`
- [x] immutable release manifest model
- [x] GitOps-only promotion intent in the model
- [x] previous-known-good rollback model
- [x] rollback policy by environment
- [x] promotion and rollback stages in the DAG

### First Pass

- [x] real local GitOps file mutation for fixture `promote_local` / `promote_dev`
- [x] real local GitOps commits for fixture `promote_local` / `promote_dev`
- [x] synthetic `observe_*`
- [x] synthetic `rollback_*`

### Remaining

- [ ] real GitOps repo mutation
- [ ] real GitOps MR creation/update/merge
- [ ] real previous-known-good tracking from Git history
- [ ] real Argo CD reconciliation integration
- [ ] real rollout observation
- [ ] real rollback observation

## Secrets / Identity

### Done

- [x] enforced `allowed_secrets`
- [x] local keychain secret resolution
- [x] GCP Secret Manager resolution
- [x] no local env-var secret override
- [x] ephemeral workspace secret mounting
- [x] runtime secret references on environment targets

### First Pass

- [x] static lookup-based secret resolution chain

### Remaining

- [ ] short-lived credential minting
- [ ] OIDC / workload identity integration
- [ ] secret access audit model
- [ ] stage-scoped cloud and GitLab identity issuance

## Concurrency / Locking

### Done

- [x] lock-key derivation for contentious stages
- [x] Postgres-backed lock manager
- [x] repo/environment lock acquisition on claim
- [x] lock refresh on heartbeat
- [x] lock release on completion or recovery

### First Pass

- [x] basic mutual exclusion on shared resources

### Remaining

- [ ] fairness under contention
- [ ] starvation mitigation
- [ ] richer lease diagnostics

## Ratchets / Invariants

### Done

- [x] finding event model
- [x] finding cluster model
- [x] invariant proposal model
- [x] active invariant model
- [x] stage-based invariant ranking
- [x] Postgres-backed ratchet store
- [x] CLI surfaces for init / ingest / rank / activate
- [x] runner injection of ranked invariants into stage context
- [x] workspace `invariants.json`

### First Pass

- [x] deterministic threshold-based proposal logic
- [x] simple stage ranking model

### Remaining

- [ ] human governance workflow for proposals
- [ ] stronger dedupe / canonicalization
- [ ] external finding ingestion from real stage tools
- [ ] materialization of approved invariants into Git-managed policy packs
- [ ] suppression / supersession / retirement lifecycle

## Stats / Cost / Timing

### Done

- [x] attempt-level stats model
- [x] run-level stats model
- [x] cost breakdown model
- [x] usage metrics model
- [x] sub-stage timing model
- [x] workspace `stats.json`
- [x] stats artifact emission
- [x] run aggregation of stage totals and sub-stage totals
- [x] issue comments include duration and cost summaries

### First Pass

- [x] deterministic scaffold cost estimation
- [x] synthetic usage/token accounting

### Remaining

- [ ] real model billing ingestion
- [ ] real scanner/tooling cost ingestion
- [ ] queue latency and scheduling delay stats
- [ ] historical baselines outside per-run state snapshots

## Signal Plane

### Done

- [x] pipeline event model
- [x] operational signal model
- [x] Postgres-backed signal store
- [x] CLI surfaces for init / list
- [x] control-plane event emission on completion, contention, and recovery
- [x] issue comment mirroring for synthesized signals

### First Pass

- [x] synthesized signals for failures, blocked stages, timing regressions, cost anomalies, contention hotspots, and stale recovery

### Remaining

- [ ] external observability ingestion
- [ ] runtime/service signal correlation
- [ ] issue synthesis from signals
- [ ] remediation / investigation run triggering
- [ ] signal closure and acknowledgement workflow

## Local / CI / Operational Topology

### Done

- [x] local Docker Compose stack
- [x] separate operational DBs for locks, ratchets, and signals
- [x] CLI surfaces for local execution
- [x] CI validation and build scaffolding

### First Pass

- [x] local parity workflow

### Remaining

- [ ] production deployment manifests for all operational services
- [ ] schema migration management
- [ ] backup / restore strategy
- [ ] monitoring and alerting for the platform itself

## Stage Implementations

### Done

- [x] stage catalog and DAG shape
- [x] stage specs and prompt packs
- [x] tooling repo placeholders

### First Pass

- [x] real local repo mutation in `implement`
- [x] structured repo/tool execution in `security` and `test`
- [x] evidence synthesis in `review`
- [x] release bundle assembly through the release plane
- [x] real local GitOps file mutation in `promote_local` / `promote_dev`
- [x] synthetic behavior still present for observation, rollback, and closeout stages

### Remaining

- [ ] real `implement`
- [ ] real `security`
- [ ] real `test`
- [ ] real `review`
- [ ] real `release_prepare`
- [ ] real `observe_local`
- [ ] real `observe_dev`
- [ ] real `promote_prod`
- [ ] real `observe_prod`
- [ ] real `rollback_dev`
- [ ] real `observe_rollback_dev`
- [ ] real `rollback_prod`
- [ ] real `observe_rollback_prod`
- [ ] real `closeout`

## Plane-by-Plane Status

### Work Plane

- `Done`: canonical issue -> work order -> run -> attempt model
- `First Pass`: fenced JSON work orders in issue descriptions
- `Remaining`: human-friendly issue translator

### Control Plane

- `Done`: queueing, retry/recovery, locking, issue mirroring, signal emission
- `First Pass`: JSON-file-backed state
- `Remaining`: durable DB-backed control-plane state and distributed scheduling

### Execution Plane

- `Done`: shared runner lifecycle, isolated workspaces, artifacts, stats, invariants
- `First Pass`: mixed real and synthetic handlers behind a registry
- `Remaining`: more stage-specific adapters and stricter capability enforcement

### Agent Plane

- `Done`: explicit `internal/agent` boundary
- `First Pass`: noop/default driver only
- `Remaining`: model-backed `plan`, `implement`, and `review` drivers

### Release Plane

- `Done`: release manifest and bundle assembly moved out of generic runner logic
- `First Pass`: still invoked through the runner's release handler
- `Remaining`: deeper Git history/GitOps awareness and standalone release stage ownership

### Repo Plane

- `Done`: shared repo discovery and source stamping helpers
- `First Pass`: local checkout resolution with synthetic fallback
- `Remaining`: remote repo operations and stronger source provenance tracking

## Recommended Next Vertical Slice

Build the first real end-to-end path in this order:

1. `implement`
2. `security`
3. `test`
4. `review`
5. `release_prepare`
6. `promote_dev`
7. `observe_dev`

That is the smallest credible lane that preserves pre-deploy security and
review while getting to a real environment outcome.
