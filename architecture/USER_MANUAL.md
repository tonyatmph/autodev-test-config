# Autodev: The Cognitive Delivery Kernel (User Manual)

## 1. Introduction: Delivery as a Compilation Problem
Autodev is not a pipeline orchestrator; it is a **recursive delivery compiler**. 

Traditional systems treat pipelines as static sequences of scripts ("Pipeline-as-a-Script"). Autodev treats delivery as an **Intent-to-State resolution problem**. You provide a `Goal` (an `Intent` + `Fitness Contract`), and the system recursively explores the `Capability Catalog` to find the sequence of immutable state transitions (the `Execution Graph`) that satisfies the contract.

The system is defined by its **"Final Form"**: **The Trinity Architecture.**

---

## 2. The Trinity: Fundamental Primitives
The system is built on the composition of three primitives that define the state machine's mechanics:

### I. The Universal Container ("The Cell")
The base unit of execution. It is a pre-baked, immutable, self-contained functional block.
- **Self-Provisioning**: It does not rely on host-side setup. It arrives at the orchestrator with its own tooling and logic baked-in.
- **Contract-Bound**: It communicates exclusively through a strict I/O contract (`context.json` for inputs, `result.json` for outputs).
- **Isolated**: It has zero knowledge of the orchestrator, the host, or other stages. It only knows its own state transition logic.

### II. The Orchestrator ("The JIT Compiler")
The engine that governs the interaction between cells. It is a domain-agnostic **Dependency Resolver**.
- **Resolver**: It evaluates the dependency graph. When a `Goal` (Intent) is required, the orchestrator triggers a `Resolve` call to find a `Provider` that satisfies the `Requirement` (Contract).
- **Enforcer (Schema/Fitness)**: It enforces the `FitnessContract` defined in the `StageSpec`. While the Provider *calculates* the fitness score, the Orchestrator *enforces* the `fitness_threshold` and triggers backtracks if requirements are not met.
- **Agnostic**: It does not perform business logic. It simply enforces contract satisfaction (Schema/Policy validation) and manages the ledger transitions.

### III. The Ledger ("Shared Memory")
The global source of truth for the system's state.
- **Content-Addressable**: Every state snapshot is a unique Git commit SHA-256 hash.
- **Transport**: Git acts as the universal transport layer for config, artifacts, and operational history.
- **Branch-as-Namespace**: Every pipeline execution lives in its own Git branch, providing perfect isolation and concurrency without locks.

---

## 3. The Operational Protocol: "Resolve, Execute, Commit"
The orchestrator drives the system through a universal, recursive loop:

1.  **Resolve**: The orchestrator evaluates the DAG. It identifies nodes where the input `SHA` is newer than the cached `Output SHA` or where the `FreshnessContract` is violated.
2.  **Execute**: It spawns the `Universal Container` (The Cell) for each node that requires a transition.
3.  **Validate**: The container performs its logic (mutation, observation, or human-gate) and commits a new `SHA` to the Ledger. The orchestrator validates the result against the schema and updates the state.

---

## 4. Composable Transaction Model
The pipeline is modeled as a **Transaction**, built incrementally across stages:
- **Transaction Object**: A single, versioned data structure representing the cumulative state of the pipeline run.
- **Atomic Application**: Each stage reads the current Transaction, mutates its specific domain surface (e.g., GitOps manifest), and writes back the updated Transaction.
- **Cascading Invalidation**: Because stages are pure functions of their inputs, any change in a parent stage automatically invalidates downstream SHAs, triggering a re-computation of the Transaction.
- **Rollback Plan**: Every Transaction contains an embedded Rollback Plan. If a stage fails (e.g., low fitness), the orchestrator triggers the rollback primitive to restore the ledger to the previous valid SHA.

---

## 5. Principles of System Design
1. **Zero-Inheritance**: Containers must not inherit ambient host state (env vars, paths).
2. **Deterministic Governance**: Transitions are driven by content-addressed SHAs, ensuring reproducibility and forensic auditability.
3. **Logic-in-Cell**: If a task requires domain-specific logic (e.g., "how to promote to dev"), it *must* reside inside a stage container, never in the orchestrator.
4. **Separation of Concerns**: 
    - **Mechanical Validation (Orchestrator)**: "Does this output match the schema?"
    - **Fitness Enforcement (Orchestrator)**: "Does the reported fitness score meet the threshold?"
    - **Fitness Calculation (Cell)**: "How do I measure the quality of this output?"
5. **Git-as-Source-of-Truth**: All operational state is indexable in Postgres, but the source of truth is always the Git Ledger.

---
*This architecture is the "Final Form." Future development must strictly adhere to these boundaries to prevent the platform from collapsing back into a legacy script-runner.*
