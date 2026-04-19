# Runtime Substrate

This directory currently holds the Git-backed execution trust anchor.

- `runtime-substrate.commit` is the only runtime trust input.
- It contains exactly one Git commit SHA.
- That commit defines:
  - the universal runtime substrate
  - stage specs
  - runtime/tooling
  - build logic
  - the expected stage image digests
- The actual image layers live in Docker/OCI storage.
- Runtime does not trust a catalog or manifest file; it derives stage image
  refs and digests from that commit.

Target end state:

- move this trust anchor into a separate hardened config repo
- build the platform against:
  - config repo identity
  - config commit SHA
- keep the same derivation rule:
  - if an image digest is not derivable from the trusted config commit, it is
    not trusted

Build the current stage image set locally with:

```sh
make build-stage-images
```

That command reads `runtime-substrate.commit`, materializes that exact Git
commit, builds the universal runtime substrate from
`docker/runner/Dockerfile`, and tags one immutable per-stage image ref from
that same substrate.

Security model:

- the universal base image is the only runtime substrate
- every stage image ref is derived from the same trusted commit
- image contents are immutable at runtime
- repo roots are mounted read-only by default
- only explicitly materialized writable paths are mounted read-write
