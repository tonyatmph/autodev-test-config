# Autodev Kernel: The Ledger-Backed State Machine (LBSM)

## 1. The Thesis
Autodev is not a CI/CD pipeline; it is an **autonomous, recursive delivery compiler**. 

Traditional orchestration is imperative and fragile—script-based pipelines that drift, leak ambient state, and break under complexity. Autodev replaces this with a **Ledger-Backed State Machine**. We treat software delivery as a formal contract-resolution problem: *How do we transition from State A to State B in a way that is verifiably compliant with a Fitness Contract?*

## 2. The Trinity: Fundamental Primitives
The entire system is derived from three fundamental primitives. **Any architectural deviation from these is a regression.**

### I. The Universal Provider ("The Cell")
The execution unit. A Provider maps `(Contract, InputSHA) -> (OutputSHA, FitnessScore)`.
- **Contract-Bound**: Communicates exclusively via structured contracts (`context.json`/`result.json`).
- **Isolationist**: Zero knowledge of the orchestrator, the host, or other stages. 
- **Self-Provisioning**: Provisions its own state from the Ledger.

### II. The Orchestrator ("The JIT Compiler")
The **Resolver Kernel** is a domain-agnostic state-transition engine. 
- **Recursive Resolver**: It resolves the dependency graph at runtime using `Goal` -> `Provider` mapping.
- **Memoization Engine**: It caches all transitions in the Ledger by `SHA`.
- **Agnostic**: It enforces contracts (Schema/Policy validation) but holds **zero business logic**. It does not know *how* to deploy or *how* to scan—it only knows how to trigger a Provider and validate the result.

### III. The Ledger ("The Provenance Store")
The global source of truth for the system's state.
- **Git-Backed**: Every successful transition is a Git commit `SHA-256`.
- **Addressable**: The current state of a pipeline is defined by a unique Git commit SHA.
- **Branch-as-Namespace**: Every execution is branch-isolated, enabling massive concurrency.

## 3. The Historical Arc
- **Phase 1 (The Refactor)**: We moved from a monolithic, script-based `runner` to a decentralized `Provider` architecture. We purged host-path dependencies (`/tmp/`) and standardized on Git-SHAs.
- **Phase 2 (The Kernel)**: We implemented the recursive `Resolver` loop that enables **JIT graph assembly**, allowing the system to self-discover dependencies.
- **Phase 3 (The Cognition)**: We integrated the LLM capability as a standard `Provider`, allowing the system to synthesize its own code mutations and verify them through recursive backtracking.

## 4. Guidance for Future Agents & Developers
This project has reached "Final Form." To maintain this integrity:

1.  **Never Add Domain Logic to the Orchestrator**: The `Resolver` kernel is domain-agnostic. If a new capability (e.g., "Deploy to AWS") is needed, **build a Provider container**, do not modify `internal/kernel/`.
2.  **Contract-First Evolution**: Any interaction between stages must be schema-validated via `internal/contracts`. If you bypass the contract, you lose auditability and predictability.
3.  **Strict Immutability**: All states must be SHA-addressable in the Git Ledger. If you introduce a system that does not record its own state as a commit, you break the forensic auditability of the kernel.
4.  **Fitness-Based Backtracking**: Use the `FitnessScore` to drive autonomous correction. A failure should never be a "Red Light"; it should be a signal for the Resolver to re-evaluate the dependency graph.

---
*This kernel is a formal delivery machine. Treat its contracts as laws.*
