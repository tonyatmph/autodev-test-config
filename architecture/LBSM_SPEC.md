# The Ledger-Backed State Machine (LBSM) Architecture

## Core Philosophy
The platform is a **ledger-backed state machine**. It replaces imperative orchestration (scripting) with a declarative **Dependency-Resolution Engine**. Business logic is strictly decoupled from the engine, delegating all domain concerns to self-contained execution primitives (the "Factories").

## 1. The Trinity: Fundamental Primitives
The system is built on the composition of three primitives that define the state machine's mechanics:

### I. The Universal Provider ("The Cell")
The base unit of execution, providing the environment for a **Provider**.
- **Contract-Bound**: It communicates exclusively through a strict I/O contract.
- **Implementation Agnostic**: A Provider can be a **Container-Cell** (Docker) or a **Native-Cell** (In-process Go).
- **Isolated**: It has zero knowledge of the orchestrator, the host, or other stages. It only knows its own state transition logic.

### II. The Orchestrator ("The JIT Compiler")
The engine that governs the interaction between cells. It is a domain-agnostic **Capability Broker**.
- **Resolver**: It traverses the dependency graph. When a `Goal` (Intent) is required, it queries the `Catalog` for a `Provider` that satisfies the `Requirement` (Contract) given the requested `Objective Domain` (e.g., 'prod', 'dev').
- **Memoization Engine**: It caches every transition `(Provider + InputSHA) -> OutputSHA` in the Ledger.
- **Agnostic**: It does not perform business logic. It simply enforces contract satisfaction and manages ledger transitions.

### III. The Ledger ("Shared Memory")
The global source of truth for the system's state.
- **Content-Addressable (SHA-256)**: Every state snapshot is identified by a unique SHA-256 hash. This is a hard architectural invariant.
- **Transport**: The Ledger is a universal protocol (Git, S3, Postgres) used to synchronize state, artifacts, and operational history.
- **Branch-as-Namespace**: Every pipeline execution lives in its own branch, providing isolation and concurrency.

---

## 2. The Operational Protocol: "Resolve, Execute, Commit"
The orchestrator operates as a **Recursive Dependency Resolver**:

1.  **Resolve**: The orchestrator receives an `Intent` + `Objective Domain`. It evaluates the DAG, querying the `Catalog` to match `Goals` to `Providers` based on constraints (latency, security, cost).
2.  **Execute**: It triggers the `Provider` (Cell), passing the `context.json`.
3.  **Validate**: The cell performs its logic (mutation, observation, or human-gate) and commits a new `SHA` to the Ledger. The orchestrator validates the result against the contract and updates the state.

---

## 3. The Capability Broker Pattern (Dynamic Linking)
The platform functions as a **distributed linker for delivery**.
- **The Catalog**: A persistent database (Postgres) that maps `Goal` names to `Provider` implementations (Container Image or Native Binary).
- **Constraint-Based Resolution**: At runtime, the orchestrator selects the optimal `Provider` implementation based on the `Objective Domain`.
- **Decoupled Logic**: Changing the implementation (e.g., swapping a Dockerized linter for an in-process native linter) requires **zero changes** to the orchestrator code or the pipeline DAG.

---

## 4. Principles of System Design
1. **Zero-Inheritance**: Providers must not inherit ambient host state. They provision themselves from the Ledger (Git SHA).
2. **Determinism as a Service**: Transitions are driven by content-addressed SHAs, ensuring reproducibility and forensic auditability.
3. **Logic-in-Cell**: Domain logic (e.g., "how to promote to dev") *must* reside within the `Provider` cell, never in the orchestrator.
4. **Contractual Governance**: The Orchestrator enforces mechanical/schema validity; the Provider provides the Fitness-Result; the Domain-Policy determines the thresholds.
5. **Git-as-Source-of-Truth**: All operational state is indexable in Postgres, but the source of truth is always the Git Ledger.

---
*This architecture is the "Final Form." Future development must strictly adhere to these boundaries to prevent the platform from collapsing back into a legacy script-runner.*









