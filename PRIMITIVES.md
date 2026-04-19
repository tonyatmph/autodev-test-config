# Primitive Ownership And Lifecycles

This document defines the ownership boundaries for Autodev’s core primitives, now standardized as an **LBSM (Ledger-Backed State Machine)** architecture.

## The Trinity

- **The Cell (Universal Container)**: The immutable execution unit.
- **The Architect (Provider)**: The recursive logic that resolves Intent to Reality.
- **The Ledger (Git SHA-Chain)**: The universal, addressable memory.

## Plane Map

- **Kernel Plane**: `internal/kernel` (Formerly `internal/runner`). Pure dependency resolution and state transition.
- **Provider Plane**: `tooling/` (Capabilities). Domain-specific implementation units.
- **Ledger Plane**: `internal/workorder`, `internal/artifacts`. Persistence and auditability.
- **Policy Plane**: `internal/policy` (now moved to Provider Cells).

## System-Level Invariants

- **Contract-First**: Every transition is governed by an I/O contract (`context.json` -> `result.json`).
- **Content-Addressable**: All state is a SHA-256 hash. No path-based reliance.
- **Recursive Resolution**: The orchestrator is a dependency solver, not a linear script runner.
- **Fitness-Driven**: Every stage transition must satisfy an explicit `Fitness Threshold`.
- **Zero-Inheritance**: Cells provision themselves from the Ledger (Git SHA).
- **Branch-as-Namespace**: Pipeline execution isolates state using Git branches.

## Primitive Lifecycle Ownership

### 1. The Kernel (Orchestrator)
- **Role**: Recursive Resolver.
- **Responsibilities**: Evaluate dependency graph, resolve missing capabilities (via Catalog), trigger Cells, validate fitness, commit SHAs to Ledger.
- **Forbidden**: No business logic, no filesystem manipulation, no secret mounting.

### 2. The Provider (Cell)
- **Role**: State Transitioner / Transformer.
- **Responsibilities**: Self-provision workspace from Ledger, apply mutation, calculate Fitness, record evidence.
- **Forbidden**: No knowledge of Orchestrator internals, no host-level ambient environment access.

### 3. The Ledger (Git)
- **Role**: Immutable Source of Truth.
- **Responsibilities**: Append-only log of transition SHAs, storage of pipeline-state graph.
- **Integrity**: Every state transition must have a corresponding SHA in the Git log.

---
*This document defines the Final Form of the Autodev Kernel. All new capabilities must conform to the Provider/Contract pattern.*
