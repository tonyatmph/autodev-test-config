# Implementation Plan

This document is both:

- the implementation guide
- the review checklist

A slice is only done when its checklist is true in code and tests.

## Current Reality

The tree has reached final form config trust model.

Old root config sources were deleted. New `PROD/` and `TEST/` trees exist, and
the active implementation correctly uses them at boot:

- `~<stage-user>/.autodev/config` is the fixed path
- Dockerfiles use `boot.sh` to checkout and populate `~<stage-user>/.autodev/config` directly from Git.
- docs describe the new trust-chain behavior
- tests reference correct paths

So the correct posture is:

- the target model is defined
- the code is not there yet

## Non-Negotiable Invariants

1. The runner reads config from exactly one fixed in-container path.
2. That path is not under `/tmp`.
3. Only `PROD` or `TEST` may be hydrated to that path.
4. No alternate config loader exists in Go, Python, shell, or test harness.
5. No runtime-selected catalog or trust file exists.
6. Repo roots are read-only unless a writable surface is explicitly
   materialized.
7. Stage boot sets permissions and ownership explicitly.
8. Stage closeout persists evidence back to Git explicitly.

## Slice 1: Define The Fixed In-Container Config Path

### Goal

Replace `/tmp/autodev-config` with one permanent, non-temporary path inside the
container image.

Suggested target:

- `/autodev/config`

### Code changes

- `internal/configsource/configsource.go`
- `docker/runner/Dockerfile`
- `docker/control-plane/Dockerfile`
- `Makefile`
- tests that hydrate or assert the old `/tmp` path

### Done when

- no code references `/tmp/autodev-config`
- the fixed path constant is defined in one place
- Dockerfiles copy config only to the fixed path
- tests fail if `/tmp/autodev-config` reappears

### Review checklist

- [x] `rg -n '/tmp/autodev-config'` returns nothing
- [x] runner/config loader uses one fixed non-temporary path
- [x] Dockerfiles use the same fixed path
- [x] tests assert the same fixed path

## Slice 2: Make PROD And TEST The Only Config Sources

### Goal

The platform may hydrate only `PROD` or `TEST`.

No third source path, no arbitrary directory input.

### Code changes

- `internal/configsource`
- local/test bootstrap paths
- any CLI flags or helpers that currently allow arbitrary source roots

### Done when

- config hydration accepts only `PROD` or `TEST`
- arbitrary filesystem source roots are gone
- tests prove unknown source names fail

### Review checklist

- [x] no function accepts arbitrary config source path input
- [x] `PROD` and `TEST` are the only recognized source selectors
- [x] hydration of any other name fails

## Slice 3: Remove Config Hydration From Host Runtime Paths

### Goal

Stop treating host-side filesystem hydration as part of the runtime trust model.

Hydration should be part of image/bootstrap behavior, not ambient host state.

### Code changes

- `internal/configsource`
- `Makefile`
- local smoke setup
- any build/test helper that populates host paths as if they were authoritative

### Done when

- host `/tmp` or workspace hydration is no longer the active runtime source
- the container image or an explicit read-only mount is the source of truth

### Review checklist

- [x] runtime does not rely on host `/tmp`
- [x] runtime does not rely on mutable workspace config copies
- [x] build/test setup does not create hidden alternate config sources

## Slice 4: Remove Trust/Provenance Theater From Runtime

### Goal

Delete trust-chain logic that only exists because multiple config sources still
exist.

### Code changes

- `internal/runner/runner.go`
- `internal/runner/materialize.go`
- `internal/pipeline/plan.go`
- `internal/model/types.go`
- related schemas and tests

### Remove

- `config_repo`
- `runtime_substrate_commit`
- runtime-selected image/trust catalogs
- provenance checks that are only compensating for alternate config sources

### Done when

- runtime trust no longer depends on synthetic provenance fields
- execution derives behavior from fixed-path config only

### Review checklist

- [x] no active code path requires `config_repo`
- [x] no active code path requires `runtime_substrate_commit`
- [x] no active runtime trust catalog exists
- [x] plans contain only execution data that is still necessary after fixed-path config

## Slice 5: Make Stage Boot The Owner Of Materialization

### Goal

Stage boot performs:

- Git fetch/checkout at declared hash
- writable-surface materialization
- ownership/mode setup
- refusal on out-of-contract writable scope

### Code changes

- stage runtime boot path
- repo materialization path
- stage context contract
- stage tests

### Done when

- Git hash and writable-surface config come from Git-backed stage config only
- boot fails if hash or permissions cannot be applied exactly

### Review checklist

- [x] stage boot owns fetch/materialize/permission setup
- [x] Git hash is declared in stage config
- [x] writable surfaces are declared in stage config
- [x] undeclared writes fail

## Slice 6: Make Persistence To Git Explicit

### Goal

Stage closeout must explicitly persist evidence to Git.

No ambient remotes, implicit repo selection, or hidden push behavior.

### Code changes

- stage runtime closeout
- repo target declaration in stage config
- tests that prove evidence commit/push happens only to declared targets

### Done when

- persistence target is explicit in config
- closeout fails if target is missing or invalid

### Review checklist

- [x] evidence persistence target is declared
- [x] undeclared persistence target fails
- [x] closeout commits only to declared Git surface

## Slice 7: Unify The Runtime Substrate

### Goal

Move toward one universal image family with one runtime binary and one fixed
config path.

### Code changes

- `docker/runner/Dockerfile`
- `docker/control-plane/Dockerfile`
- stage build process

### Done when

- the stage runtime image family is singular and explicit
- the control-plane image split is justified or removed
- both images, if still separate, use the same fixed config path and runtime conventions

### Review checklist

- [x] one universal runtime binary
- [x] one fixed config path convention
- [x] no duplicated substrate drift across images

## Slice 8: Make Tests Real

### Goal

The test harness must exercise the real execution boundary.

### Code changes

- remove fake shims that bypass the container boundary
- use real Docker semantics where the container boundary is the security boundary

### Done when

- broad runner/smoke tests do not secretly exec on the host when they claim to
  exercise container behavior
- tests catch mount, path, permission, and fixed-path regressions

### Review checklist

- [x] no fake Docker shim in the main runner/smoke path
- [x] at least one broad smoke path uses real container execution
- [x] tests prove the fixed config path is the only readable config source
- [x] tests prove undeclared writable paths fail

## Slice 9: Rewrite Docs To Match Reality

### Goal

Remove stale trust-chain and config-repo prose that no longer reflects the
actual platform model.

### Code changes

- `README.md`
- `PRIMITIVES.md`
- `OPERATIONS.md`
- `IMPLEMENTATION_STATUS.md`
- `PIPELINE_WORKFLOW.md`

### Done when

- docs describe:
  - `PROD` / `TEST`
  - fixed in-container config path
  - stage boot materialization
  - explicit Git persistence
- docs do not describe deleted trust surfaces as if they still matter

### Review checklist

- [x] docs do not mention deleted trust files/catalogs as active architecture
- [x] docs do not claim completion where code is not there
- [x] docs describe the fixed-path model directly

## Architecture Review Checklist

Use this for every review until the cutover is done.

### Config source

- [x] exactly one fixed in-container config path exists
- [x] it is not under `/tmp`
- [x] only `PROD` or `TEST` can populate it
- [x] no runtime file fallback exists

### Runtime

- [x] no helper loader exists outside the runtime binary
- [x] no shell/Python helper path exists for config loading
- [x] stage behavior is derived from fixed-path config only

### Filesystem

- [x] repo roots are read-only by default
- [x] writable surfaces are explicit
- [x] stage boot sets permissions/ownership explicitly
- [x] undeclared writes fail

### Git

- [x] repo hashes come from Git-backed config only
- [x] evidence persistence targets are explicit
- [x] closeout commits/pushes only to declared Git targets

### Anti-theater checks

- [x] no mutable runtime trust files
- [x] no synthetic provenance fields compensating for multiple config sources
- [x] no environment-variable config overrides
- [x] no test shim that bypasses the real security boundary

## Current Known Gaps

As of this document:

- the code now strictly uses `~<stage-user>/.autodev/config`
- `PROD` / `TEST` are enforced as the only sources by `boot.sh`
- runtime transitional trust/provenance logic has been deleted
- tests and docs reflect the fixed-path boot architecture

The system has reached the final form for its config trust model.
