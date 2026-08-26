# Changelog

Notable changes to herald-dingtalk are documented in this file.

The project follows [Semantic Versioning](https://semver.org/). Release tags include the `v` prefix, while `herald-dingtalk --version` prints the version without it.

## [1.0.0] - 2026-08-26

v1.0.0 is the first stable release. It consolidates the HTTP provider contract, deployment safeguards, and release process introduced during the v0.x series.

### Added

- Optional API key authentication for provider endpoints.
- Idempotent send requests with concurrent request coalescing and successful-result caching.
- DingTalk OAuth2 code resolution and optional mobile-to-userid lookup.
- `/healthz` liveness and configuration-aware `/readyz` readiness endpoints.
- Per-process request concurrency limits, bounded request bodies, request IDs, security headers, and structured logs.
- Multi-platform release binaries, SHA256 checksums, multi-architecture container images, SBOM, and provenance metadata.
- `--version` output backed by release-time version metadata.

### Changed

- Fiber is now v3.5.0; Fiber-facing internal kit dependencies use their stable v2 releases.
- Requests without `timeout_seconds` use a 25-second end-to-end deadline. The accepted maximum remains 30 seconds, and graceful shutdown allows up to 35 seconds for in-flight requests.
- Idempotent operations no longer inherit the first caller's cancellation. Each caller may stop waiting independently while the shared provider operation remains bounded by its own timeout.
- Idempotency cache expiration and capacity eviction use ordered O(1) removal. The cache remains process-local and is not a cross-replica deduplication mechanism.
- DingTalk failures are mapped to stable client-facing categories without exposing upstream response details.

### Fixed

- Release and CI linker paths now target `version-kit/v2`, and builds verify the embedded version before publishing artifacts.
- Empty mobile lookup results retain `invalid_destination` classification.
- Provider calls, OAuth resolution, and shutdown now have consistent bounded lifetimes.
- Failed idempotent operations remain retryable and panicking operations unblock waiters safely.

### Security

- GitHub Actions and the `golangci-lint` and `govulncheck` tool versions are pinned to reduce supply-chain drift.
- The container runs as an unprivileged user, and production guidance requires secrets to be injected rather than committed.

### Upgrading from v0.7.0

- Review retry logic if it depends on broad legacy status handling. Stable categories now include `400 invalid_destination`, `429 rate_limited`, `502 send_failed` or `resolve_failed`, `503 provider_down`, and `504 timeout`.
- Treat `invalid_destination` and invalid requests as non-retryable. Honor `Retry-After` for local concurrency limits; use the same idempotency key when retrying rate limits, upstream failures, or timeouts.
- Allow at least 40 seconds of container or orchestrator termination grace so the server's 35-second shutdown window can complete.
- Use `/healthz` for liveness and `/readyz` for readiness. Readiness validates local configuration only and does not call DingTalk.
- Do not assume idempotency works across replicas. Keep a single replica or add deterministic external routing/deduplication when cross-replica guarantees are required.

[1.0.0]: https://github.com/soulteary/herald-dingtalk/compare/v0.7.0...v1.0.0
