# Operations Guide

This document is the practical operator guide for the current scaffold.

For ownership boundaries and allowed mutators per primitive, see
[PRIMITIVES.md](/Users/tony/mph.tech/worktrees/codex/autodev/PRIMITIVES.md).

## Current Operating Modes

Autodev can run in two modes:

- `file-backed local mode`
  - issues are JSON files under `data/gitlab/issues`
  - useful for local smoke tests and CI
- `real GitLab mode`
  - issues are read from your self-hosted GitLab issues project
  - comments and labels are written back through the GitLab API

In both modes, the control plane uses the same run/attempt lifecycle and the
same typed stage contract.

## Documentation As A Delivery Surface

Documentation is now a first-class delivery surface. Treat `docs` the same way
you treat `api`, `console`, `config`, `prompts`, or `mobile` when the change
affects behavior or operations.

First-pass enforcement now exists in the synthetic pipeline:

- a work order can declare documentation as required
- it can designate which component key represents docs, usually `docs`
- `review` and `release_prepare` will block if that docs surface is required
  but not included in `selected_components`

This is not yet a real content-diff validator. It does not prove the docs repo
was updated correctly. It does enforce that documentation is part of the
delivery object and therefore part of the review/promotion path.

## Governed Surfaces and Identities

Autodev enforces three writable identities: `agent`, `generator`, and `governed`. The governed identity alone may edit prompts, stage specs, policies, and other platform guardrails; agents and generators treat those paths as read-only references. When applying governed changes, run the commits or CI job as the governance identity so the platform can audit who touched its rules and prompts.

The local fixture lane now declares which governed surfaces it relies on so the identity rules apply end-to-end. Keep using `hack/local-sample-issue.json` and the `local-gitops` checkout to validate that the governed surfaces remain untouched by the agent slices.

Identity-bound stages now execute behind an in-process sandbox:

- file writes are limited to identity-owned path globs plus the stage workspace
- Git-mutating repo control is limited to repos explicitly owned by that identity
- `implement` writes only into agent-owned authored surfaces
- `generate` writes only into generator-owned materialized surfaces
- `promote_*` and `rollback_*` write only into governed GitOps environment paths

Long-term operating assumption:

- durable work-order, generated, governed, journal, and GitOps changes should
  be committed into their respective Git surfaces
- runtime orchestration state stays in Postgres-backed operational stores
- each identity should execute under a separate OS/container user with matching
  credentials and checkout ownership

The stage primitive executes through digest-pinned stage images derived only
from:

- stage name
- `containers/runtime-substrate.commit`

The run plan records the `runtime_substrate_commit`, the derived image ref, and
the derived digest. The launcher refuses to run if the plan does not declare
that commit or if the recorded image ref/digest do not match what is derivable
from that commit.

Target operating model:

- the runtime trust anchor should move to a separate governed config repo
- the platform binary should be built against:
  - config repo identity
  - config commit SHA
- runtime should trust only that baked config commit
- `main` in the config repo is the governance baseline
- each pipeline is a branch derived from `main`
- upgrades are handled by rebasing pipeline branches onto newer `main`,
  rebuilding, and validating legacy vs upgraded pipelines in parallel

Operational boundary rule:

- config is the contract
- Go propagates the contract and manages stage lifecycle
- the container runtime/OS enforce runtime identity and filesystem/network
  boundaries
- the universal container primitive owns all in-container behavior
- repo roots are read-only by default
- only explicitly materialized writable paths are read-write
- stage commands resolve against fixed interpreter/tool paths inside the
  universal runtime substrate, not ambient host or container PATH

Go should never implement or patch stage business logic directly.

## Config Files

Autodev now treats versioned config files as the only supported runtime config
surface. The default local file is
[autodev.config.json](/Users/tony/mph.tech/worktrees/codex/autodev/autodev.config.json).
The real-GitLab local/e2e file is
[autodev.gitlab.json](/Users/tony/mph.tech/worktrees/codex/autodev/autodev.gitlab.json).
Docker Compose uses
[autodev.compose.json](/Users/tony/mph.tech/worktrees/codex/autodev/autodev.compose.json).
Kubernetes manifests mount
[autodev.k8s.json](/Users/tony/mph.tech/worktrees/codex/autodev/autodev.k8s.json)
through a ConfigMap.

Use `--config <path>` with both binaries:

```sh
./bin/control-plane --config autodev.config.json snapshot
./bin/stage-runner --config autodev.config.json local --issue hack/sample-issue.json
```

The key config sections are:

- `paths`
  - `root_dir`
  - `data_dir`
  - `spec_dir`
  - `work_order_repo`
  - `repo_roots`
  - `smoke_secrets`
- `gitlab`
  - `base_url`
  - `issues_project`
  - `token`
  - `token_name`
- `stores`
  - `locks_postgres_dsn`
  - `ratchet_postgres_dsn`
  - `signals_postgres_dsn`
- `secrets`
  - `gcp_project`
  - `local_keychain_service`
- `execution`
  - no mutable runtime image selector; stage images are derived from
    `containers/runtime-substrate.commit`

Secret lookup order is still:

1. local keychain
2. GCP Secret Manager

There is intentionally no environment-variable override path for runtime
configuration or secrets.

## Local Commands

### Build

```sh
make build
```

### Build reusable stage images

```sh
make build-stage-images
```

This reads
[containers/runtime-substrate.commit](/Users/tony/mph.tech/worktrees/codex/autodev/containers/runtime-substrate.commit),
materializes that exact Git commit, builds the universal runtime substrate from
`docker/runner/Dockerfile`, and tags one immutable per-stage image ref from
that same substrate.

Target end state:

- the trust anchor should be the baked config-repo commit, not a mutable file
  in the platform repo
- the derivation/build shape stays the same
- only the source of the trusted commit changes

### Test

```sh
make test
```

### Start local stack

```sh
make local-up
```

The Compose stack provides:

- `control-plane`
- `locks-db`
- `ratchet-db`
- `signals-db`
- `worker-intake`
- `worker-implement`

### Stop local stack

```sh
make local-down
```

### Run a local smoke issue

```sh
make sample-run
```

This seeds [hack/sample-issue.json](/Users/tony/mph.tech/worktrees/codex/autodev/hack/sample-issue.json)
through the local file-backed mode and runs until completion, failure, or
approval wait. By default, `stage-runner local` uses the deterministic fixture
[hack/smoke-secrets.json](/Users/tony/mph.tech/worktrees/codex/autodev/hack/smoke-secrets.json)
for required stage secrets such as `gitlab-write-token` and
`gitops-write-token`.

For the local fixture lane, run `bash hack/init-e2e-fixture-repos.sh`, include
`/tmp/autodev-e2e-repos` in `paths.repo_roots`, and invoke `stage-runner local`
against [hack/local-sample-issue.json](/Users/tony/mph.tech/worktrees/codex/autodev/hack/local-sample-issue.json).
That issue now targets the real Git-backed e2e fixture repos instead of the
old in-tree local repo convention.

This local fixture lane is the intended **Stage 1 dev validation path**:

- real GitLab issue optional, local pipeline strongly preferred
- real local repo checkouts
- real local Git commits for work orders, generated outputs, and GitOps files
- no real infra deployment required

### Meta-pipeline validation

`make meta-validate` first builds the reusable stage images (`make build-stage-images`) from
`containers/runtime-substrate.commit`, then it runs
`stage-runner` with [autodev.meta.json](/Users/tony/mph.tech/worktrees/codex/autodev/autodev.meta.json)
against [hack/e2e-pipeline-issue.json](/Users/tony/mph.tech/worktrees/codex/autodev/hack/e2e-pipeline-issue.json)
in container mode. This verifies the materialized pipeline plan using the same
runtime substrate commit the images were built from, and the container writes durable artifacts that the
control plane journals into the run log.

This creates a reproducible self-host validation lane:

- run the meta-pipeline job locally with `make meta-validate`
- CI runs the same command inside `gitlab/pipeline.yml`'s `build-stage-images` job
- failures surface as pipeline regressions against the new stage images
- fast iteration before wiring the remote dev lane

There is now a dedicated reusable e2e fixture too:

- initialize the fixture repos with `bash hack/init-e2e-fixture-repos.sh`
- local checkouts land under `/tmp/autodev-e2e-repos/` by default
- the real app repo is `mph-tech/autodev-e2e-app`
- the matching GitOps repo is `mph-tech/autodev-e2e-gitops`
- the real GitLab issue template lives at [hack/e2e-issue-template.md](/Users/tony/mph.tech/worktrees/codex/autodev/hack/e2e-issue-template.md)

These fixture repos are source inputs only. The universal stage primitive
materializes run-scoped Git repos under `paths.data_dir/repos/<run-id>/...`
before stages read or mutate them, so the e2e lane uses the same repo-init
shape as production.

When you need a fully repeatable test lane, pin `ref` fields in the work order
for app, journal, and GitOps repos. The universal materializer will check out
those exact SHAs in the run-scoped repos.

To run the Stage 1 lane locally against a **real GitLab issue** instead of a
file-backed issue:

```sh
./bin/stage-runner --config autodev.gitlab.json local --issue-id gitlab:7:1 --smoke-secrets hack/smoke-secrets.json
```

`--issue-id` keeps the single-process local loop focused on one real GitLab
issue while still using local repos, local GitOps, and the local journal.

To force the same lane through the real universal container primitive:

```sh
./bin/stage-runner --config autodev.config.json local --issue hack/local-sample-issue.json --smoke-secrets hack/smoke-secrets.json
```

The control plane threads any journal metadata that the stages expose into the
run and attempt records (`journal_entry`, `journal_history`, `last_journal`),
so you can inspect the same JSON files or the state store to see component commit
SHAs, release manifest summaries, and promotion plan details without chasing
artifacts manually.

The work-order repo now contains the canonical full reports for each stage
attempt plus a run-level index:

- `work-orders/<work-order-id>/work-order.json`
- `work-orders/<work-order-id>/runs/<run-id>/index.json`
- `work-orders/<work-order-id>/runs/<run-id>/stages/<stage>/attempt-XX/summary.json`
- `work-orders/<work-order-id>/runs/<run-id>/stages/<stage>/attempt-XX/report.json`

Artifacts remain useful, but they are now auxiliary:

- artifact storage holds raw logs, bulky outputs, and metadata
- the work-order repo holds the full reports that stages and humans should read

Current readers of the durable journal:

- `review` reads `test` and `security` reports from the work-order repo first
- `release_prepare` reads `test`, `security`, and `review` reports from the
  work-order repo first
- both still fall back to the local artifact cache if the durable report is not
  present yet

The current durable metadata keys to expect in state are:

- `run.Metadata["work_order_commit"]`
- `attempt.Metadata["journal_entry"]`
- `run.Metadata["journal_history"]`
- `run.Metadata["last_journal"]`
- `attempt.Metadata["generator_commit"]`
- `run.Metadata["generator_commits"]`
- `attempt.Metadata["promotion_gitops_commit"]`
- `run.Metadata["promotion_commits"]`
- `attempt.Metadata["stage_report"]`
- `run.Metadata["run_index"]`

The current machine-readable stage outputs to expect are:

- `mr_proposal` from `implement`
- `merge_requests` from `implement` when GitLab MR upsert succeeds
- `generator_journal` and `generator_commit` from `generate`
- `promotion_plan`, `promotion_commit`, and `promotion_gitops_commit` from
  `promote_*`
- `merge_request` from `promote_*` when GitLab MR upsert succeeds

## Control Plane CLI

### Intake and reconcile

```sh
./bin/control-plane enqueue
./bin/control-plane reconcile
```

### Recover stale attempts

```sh
./bin/control-plane recover-stuck-runs
```

### Snapshot state

```sh
./bin/control-plane snapshot
```

### Serve HTTP API

```sh
./bin/control-plane serve --addr :8080
```

### Use the operator dashboard

With the control plane serving, open:

- [http://localhost:8080/assets/index.html](http://localhost:8080/assets/index.html)

The dashboard supports:

- refreshing GitLab issues
- importing a local issue contract for testing
- materializing a run for an issue
- issue-scoped reconcile
- viewing live run/stage state
- viewing the Git-backed run index and latest stage report

The dashboard drives the same control-plane actions as the CLI. Stage progress
still depends on local execution or workers claiming attempts.

## Stage Runner CLI

### Worker mode

```sh
./bin/stage-runner worker --control-plane-url http://127.0.0.1:8080
```

Restrict to certain stages:

```sh
./bin/stage-runner worker --control-plane-url http://127.0.0.1:8080 --stages intake,plan,review
```

Run one claim loop only:

```sh
./bin/stage-runner worker --control-plane-url http://127.0.0.1:8080 --once
```

### Execute one stage from persisted state

```sh
./bin/stage-runner run --stage implement --run-id run-1
```

This command is still orchestration-only: it prepares the stage context and
invokes the configured universal container entrypoint for the stage. Go does
not execute the stage business logic itself.

### Local file-backed end-to-end

```sh
./bin/stage-runner local --issue hack/sample-issue.json
```

To disable the smoke fixture and require the normal keychain/GCP chain:

```sh
./bin/stage-runner local --issue hack/sample-issue.json --smoke-secrets ""
```

## Real GitLab Operation

Populate the `gitlab` section in the active config file, for example:

`autodev.gitlab.json` already provides the internal baseline. If you need a
different project or host, edit the `gitlab` section there or create another
versioned config file:

```json
{
  "gitlab": {
    "base_url": "http://10.142.0.2/api/v4",
    "issues_project": "mph-tech/autodev-issues",
    "token_name": "gitlab-token"
  },
  "secrets": {
    "local_keychain_service": "autodev"
  }
}
```

If you do not want keychain lookup, set `gitlab.token` directly in a
non-committed local config file.

### Human issue workflow

1. Create a GitLab issue in the configured issues project.
2. Add a delivery label such as `delivery/requested`.
3. Put the canonical work-order JSON into a fenced `json` block in the issue description.
4. Run:

```sh
./bin/control-plane enqueue
./bin/control-plane reconcile
```

5. Start workers.
6. Watch comments and labels on the issue for progress.

Current limitation:

- the issue description is still expected to contain the machine work order
- the human-friendly issue-to-work-order translator is not implemented yet
- docs enforcement currently checks that the docs surface is included when
  required; it does not yet validate actual docs diffs or runbook completeness
- the first real `implement` slice resolves actual local component commit SHAs
  when a checkout can be found under `paths.repo_roots`, but it does not yet
  create branches, commits, or merge requests in GitLab

## Ratchet Operations

Initialize ratchet tables:

```sh
./bin/control-plane ratchet-init
```

Ingest a sample finding:

```sh
./bin/control-plane ratchet-ingest --file hack/sample-finding.json
```

Retrieve top invariants for a stage:

```sh
./bin/control-plane ratchet-top --stage implement --repo-scope platform/example-service --environment dev --service-scope example-service --limit 5
```

Activate a proposal:

```sh
./bin/control-plane ratchet-activate --proposal-id 1 --enforcement-mode warn
```

## Signal Operations

Initialize signal tables:

```sh
./bin/control-plane signal-init
```

List open signals:

```sh
./bin/control-plane signal-list --status open --limit 20
```

List stage-specific signals:

```sh
./bin/control-plane signal-list --repo-scope mph-tech/example-service --stage implement
```

## Artifacts and State

Default local paths under `paths.data_dir`:

- `state/`
  - persisted runs, attempts, issue mirrors
- `artifacts/`
  - emitted result/evidence/stats/release-manifest artifacts
- `gitlab/issues/`
  - file-backed issue cache
- `workspaces/<run>/<stage>/`
  - prompt copy
  - tooling copy
  - resolved secret files
  - `invariants.json`
  - `stats.json`

## What Is Real vs Synthetic

### Real enough to operate now

- issue ingestion and reconciliation
- run and attempt lifecycle
- worker claim/heartbeat/complete
- lock management
- secret resolution
- ratchet persistence and retrieval
- signal persistence and synthesis
- stats, cost, and timing rollups
- first-pass real GitLab issue/comment/label integration
- work-order and component dependency modeling

### Still synthetic

- most stage bodies
- GitOps repo mutation
- rollout observation
- rollback observation
- per-component Git commit stamping
- remote MR creation and remote repo writes
- documentation content verification

## Smoke-Test Recommendation

For the first meaningful end-to-end slice, build out these stages in order:

1. `implement`
2. `security`
3. `test`
4. `review`
5. `release_prepare`
6. `promote_dev`
7. `observe_dev`

That gives you:

`issue -> work order -> code change -> security/test/review -> release manifest -> dev promotion -> dev observation`

## Troubleshooting

### No issues are ingested

Check:

- `delivery/*` label is present
- GitLab issue project matches `gitlab.issues_project` in the active config
- issue description contains a fenced `json` block

### GitLab token resolution fails

Either:

- set `gitlab.token` in a local config file
- or ensure the token exists in the local keychain under the configured service/name

### Ratchet or signal commands fail on startup

Check:

- `stores.*_postgres_dsn` is set in the active config
- the target Postgres instance is reachable
- the initialization command has run

### A run stalls

Inspect:

- `./bin/control-plane snapshot`
- `./bin/control-plane signal-list`
- `./bin/control-plane recover-stuck-runs`

### Stage context looks wrong

Inspect the workspace:

- `data/workspaces/<run>/<stage>/invariants.json`
- `data/workspaces/<run>/<stage>/stats.json`

and the artifact payloads under `data/artifacts/`.
