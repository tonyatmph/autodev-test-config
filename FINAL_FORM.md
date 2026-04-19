# Final Form

This document defines the spec final-form platform model.

Anything in the codebase that violates these rules in sprit or practice is forbidden and an invariant.

## Trust Model

There is exactly one trust anchor: `runtime-substrate.commit`.

That commit pins, by content, everything the runtime is allowed to load:

- base image definition
- stage image definitions
- runtime binary and tooling
- the identity of the config source — which config repo, at which commit,
  over which transport, with which credentials and host-key trust

From that single commit, the runtime derives:

- image digests for the substrate
- the exact config repo and commit to pull at boot

The runtime accepts only:

- a declared substrate commit `C`
- image digests derived from `C`
- one config tree pulled at boot from the config source declared by `C`

Stage specs and pipeline config are not embedded in `runtime-substrate.commit`.
They live in the config repo. The substrate commit pins only the *identity* of
that repo; the repo pins the *content*. Together they form a single, fully
content-addressed trust chain rooted at `runtime-substrate.commit`.

Nothing outside what `C` pins is trusted. There is no separate trust catalog,
no runtime provenance input, and no operator-supplied override.

## Core Rule

Autodev is a workflow orchestrator that runs inside a fixed container
substrate and reads workflow configuration from exactly one fixed in-container
path.

There are only two possible config sources in the platform:

- `PROD` git config repo
- `TEST` git config repo

At runtime, the boot script pulls exactly one of them into the container over
git before the runner starts. They are never both in existence in a stage or
in autodev. Both are backed and distributed by git.

Nothing else is a config source. Everything else is strictly prohibited.

## Config Source Selection

The runner does not choose `PROD` or `TEST`.

That choice is made upstream and pinned by `runtime-substrate.commit`. The
deployment artifact produced from that commit declares which config repo and
commit the boot script pulls:

- a `PROD` deployment artifact pulls `PROD`
- a `TEST` deployment artifact pulls `TEST`

Inside the container, the runner has no:

- CLI flag
- environment variable
- runtime branch
- helper override

that selects between them.

The runner sees exactly one config tree at one fixed path and reads only that
tree.

## Fixed Config Model

The runtime model is:

1. start the Autodev container
2. the boot script pulls exactly one config tree, `PROD` or `TEST`, to
   `~<stage-user>/.autodev/config` before the runner starts, using the
   source identity pinned by `runtime-substrate.commit`
3. read stage specs and pipeline definitions only from that location
4. derive stage execution from those files only

There is no:

- runtime-selected config path
- embedded alternate config source
- helper-script config loader
- runtime trust catalog
- operator-supplied provenance input
- environment-variable config override

If the runner can read only one source, it does not need to perform runtime
provenance theater to distinguish among multiple possible sources.

The fixed config path must satisfy all of these constraints:

- it is materialized at `~<stage-user>/.autodev/config`
- it is not under `/tmp` or any other ambient path
- it is mounted or copied read-only
- it remains immutable for the lifetime of the container

No stage may mutate the config tree after startup. The runner does not copy,
hydrate, or rewrite that path. The deployment artifact fixes the stage user
identity together with the config tree. The runner does not choose, discover,
or switch the stage user before reading the fixed config path.

## Container Model

Autodev runs in one universal container image family, whose digests are
derived from `runtime-substrate.commit`.

Properties:

- immutable image
- fixed runtime binary
- fixed in-container config path
- no shell-script helper path
- no Python-script helper path
- no helper loader in any language, including statically compiled Go
- no fallback loader

Stages run as separate short-lived containers launched by the orchestrator,
but they all use the same runtime substrate shape.

The control plane may eventually be folded into the same image family if that
reduces drift and duplicated substrate management.

## Stage Model

The orchestrator is generic.

It should not care whether a stage is:

- a container command
- an HTTP call
- a queue job
- a human approval
- an SMS send
- a voice call

Every stage must normalize into the same observable contract:

- identity
- dependencies
- inputs
- outputs
- success criteria
- normalized lifecycle state
- evidence

The orchestrator consumes only that normalized contract/result shape.

## Boot / Execute / Collect / Persist Lifecycle

Each stage follows the same lifecycle:

1. **Boot**
   - read stage config from the fixed read-only path
     `~<stage-user>/.autodev/config`
   - fetch declared repos from declared remotes at declared hashes
   - materialize writable surfaces under explicitly declared writable
     subdirectories inside the running stage user's home directory
   - set ownership and permissions
   - fail if surfaces exceed contract
   - fail if any writable path outside the declared writable subdirectories is
     not explicitly whitelisted in Git-backed stage config

2. **Execute**
   - run as the declared local/container user
   - read from mounted inputs
   - write only inside declared materialized writable surfaces

3. **Collect**
   - gather result, report, state, and evidence
   - validate before persistence

4. **Persist**
   - commit/push evidence back to declared Git targets only

Git does not preserve full filesystem permissions. Materialization must
therefore set ownership and mode explicitly. That behavior belongs to stage
boot, not to Git.

## Stage User Model

Every stage runs as a real stage user account.

The stage user's home directory is the containment boundary, but only
explicitly declared subdirectories within it are writable.

That means:

- stage config is materialized under `~<stage-user>/.autodev/config`
- stage workspaces are materialized under `~<stage-user>/workspace`
- stage-local writable surfaces are materialized under
  `~<stage-user>/materialized`
- stage artifacts are materialized under `~<stage-user>/artifacts`

Everything outside the running stage user's home directory is write-protected
unless it is an explicitly mounted platform-provided path declared in
Git-backed stage config.

The orchestrator and runtime treat:

- the stage user's home directory as the containment boundary
- only explicitly declared writable subdirectories inside that home as
  writable
- the config subtree inside that home directory as always read-only
- explicit Git-backed whitelist rules as the only way to widen writes beyond
  those writable subdirectories
- writes outside `~<stage-user>` as allowed only to explicitly mounted
  platform-provided paths declared in stage config
- arbitrary system paths, other users' homes, and undeclared mounts as never
  writable
- paths outside `~<stage-user>` as absent from the stage container unless
  explicitly mounted by the platform
- undeclared external paths as not visible, not just non-writable

There is no ambient writable host or container area outside that rule.

## Git Transport Rules

Git transport is explicit and contract-driven.

Fetch and push may use only:

- declared remote identity
- declared credentials
- declared host-key trust

All three are pinned, directly or transitively, by `runtime-substrate.commit`.
Ambient transport state is not trusted.

The fixed config source is declarative only. It may not provide executable
transport helpers. Fetch and push use only the transport binaries built into
the fixed runtime image.

Fetch and push run non-interactively. If the transport binaries request user
input, credential entry, host confirmation, or any other interactive response,
the operation fails immediately.

That means the runtime ignores:

- inherited Git remotes
- ambient Git configuration
- ambient SSH configuration
- undeclared credentials
- ambient credential helpers
- ambient askpass helpers
- ambient SSH command wrappers
- undeclared transport helper discovery

If the remote, credential, or host key does not match the stage contract,
fetch/push fails.

## Filesystem Rules

The container image is immutable.

Writable state exists only in materialized write surfaces owned by the
running user.

Rules:

- repo roots are read-only by default
- writable subpaths are explicit
- writes outside declared materialized surfaces fail
- runtime config is not read from host `/tmp`
- runtime config is not read from ambient working tree paths
- the fixed config path is read-only for the lifetime of the container
- writable surfaces are always separate from the fixed config tree
- the running stage user's home directory is not writable as a blanket rule
- only declared writable subdirectories under the running stage user's home
  are writable
- anything outside that home directory is write-protected unless it is an
  explicitly mounted platform-provided path declared in Git-backed stage
  config
- arbitrary system paths, other users' homes, and undeclared mounts are never
  writable
- paths outside the running stage user's home are absent unless explicitly
  mounted by the platform
- undeclared external paths are not visible, not just non-writable
- mutation of the fixed config tree is a hard failure

## Why This Model

The simplest valid trust model here is not "provenance logic everywhere."

It is:

- one fixed substrate commit (`runtime-substrate.commit`)
- one fixed container substrate derived from that commit
- one fixed config source declared by that commit (`PROD` or `TEST`)
- one fixed read-only config path inside the container

If nothing else is visible to the runner, then there is nothing else to
trust. That is stronger than runtime trust theater.

This model exists to eliminate:

- split-brain config
- runtime-selected config files
- trust catalogs
- baked-in fallback loaders
- helper scripts that become alternate execution paths
- security controls that only look strong because they add more checks on top
  of a weak source model

The goal is to make the system smaller and more provable, not more
decorated.

## Workflow Engine Direction

Today the platform is used for software delivery.

The final form is broader:

- any workflow can be modeled as stages
- every workflow becomes observable
- every workflow becomes instrumentable
- every workflow becomes governable
- every workflow becomes automatable

The ratchet/feedback loop comes from:

- explicit workflow definitions
- explicit evidence
- explicit scoring
- explicit comparison across runs/pipelines/strategies

That is how the system evolves workflows without turning orchestration into a
black box.

## What Is Not Part Of Final Form

These are explicitly forbidden as part of the intended model:

- runtime-selected trust files
- mutable runtime config directories
- alternate, fallback, or backup config sources baked into the platform repo
- helper loaders of any kind — including statically compiled Go loaders —
  that embed or re-hydrate config from the platform repo
- multiple config sources existing at once
- any trust anchor other than `runtime-substrate.commit`

If the runner can read only one fixed config path inside the container, most
of that machinery should disappear.
