# Changelog

Notable changes to herald-dingtalk are documented in this file.

The project follows [Semantic Versioning](https://semver.org/). Release tags include the `v` prefix, while `herald-dingtalk --version` prints the version without it.

## [Unreleased]

## [1.2.8] - 2026-11-13

### Changed

- Require Go 1.27.4 (previously 1.27.3) in `go.mod` and build the container
  image with `golang:1.27.4-alpine3.24`. Go 1.27.4 is a security release and
  includes the net/http fix for GO-2026-6859.
- Update Fiber from v3.6.1 to v3.7.0.
- Update kit modules: cli-kit v1.9.3 to v1.9.4, health-kit/v4 v4.0.5 to v4.0.7,
  logger-kit/v3 v3.0.4 to v3.0.6, provider-kit v1.7.4 to v1.7.5 and
  version-kit/v4 v4.0.3 to v4.0.5.
- Refresh indirect modules: gofiber/schema v1.9.2 to v1.9.3, gofiber/utils/v2
  v2.6.3 to v2.6.4 and fasthttp v1.77.0 to v1.78.0.
- This release contains no changes to herald-dingtalk source code; the HTTP
  API, configuration and delivery behavior are the same as in v1.2.7.

## [1.2.7] - 2026-11-02

### Tests

- Verify real process exit on bind failure, listener failures before/after
  shutdown, and shutdown deadlines with a blocked HTTP request.
- Synchronize token-refresh waiters before cancellation and verify upstream
  timeouts do not trigger an unintended retry; cover provider error boundaries.
- Raise the CI coverage floor from 95% to 98%; Linux race tests reach 98.0%
  statement coverage without excluding production code. Production behavior
  and delivery guarantees are unchanged.

## [1.2.6] - 2026-11-02

### Fixed

- Log stable provider error categories instead of raw network URLs and upstream
  error text that can contain application credentials, access tokens or payloads.
- Use a fixed message for malformed JSON bodies in send and resolve handlers.
  Existing logs and exports require separate access/retention review and rotation
  of any exposed credentials; see the security guide.


### Tests

- Cover process startup, version output, SIGTERM shutdown, rejected-token refresh,
  transport failures and idempotency coordination failures. Enforce at least
  95% total statement coverage in CI. Production behavior is unchanged.

### Changed

- Include the previously merged health-kit dependency alignment and verified kit
  maintenance upgrades; refresh required transitive dependencies and checksums.

## [1.2.5] - 2026-10-22

### Changed

- Require Go 1.27.3 and align build images and CI toolchains.
- Update compatible Go dependencies and published Kit versions; refresh module checksums.
- Preserve the project toolchain after quality report generation.


## [1.2.3] - 2026-09-21

### Changed

- Upgraded the internal kit dependencies to their latest major releases: `health-kit` v2.3.0 to v4.0.0, `logger-kit` v2.3.0 to v3.0.0, and `version-kit` v2.2.0 to v4.0.0. Each new major line moves its Fiber-facing API into a `fiberadapter` subpackage, so the access log middleware, the request ID lookup, and the `/healthz` handler are now sourced from `logger-kit/v3/fiberadapter` and `health-kit/v4/fiberadapter`. `cli-kit` and `provider-kit` are unchanged.
- Release, CI, and Dockerfile linker paths now target `version-kit/v4`. A stale `-X` path is not a build error, so a missed path would have silently shipped binaries reporting `dev`.
- Service behavior is unchanged: access log fields including `request_id`, the `/healthz` response body, and all routes are identical to the previous release.

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

[Unreleased]: https://github.com/soulteary/herald-dingtalk/compare/v1.2.8...HEAD
[1.2.8]: https://github.com/soulteary/herald-dingtalk/compare/v1.2.7...v1.2.8
[1.2.7]: https://github.com/soulteary/herald-dingtalk/compare/v1.2.6...v1.2.7
[1.2.6]: https://github.com/soulteary/herald-dingtalk/compare/v1.2.5...v1.2.6
[1.2.5]: https://github.com/soulteary/herald-dingtalk/compare/v1.2.4...v1.2.5
[1.2.3]: https://github.com/soulteary/herald-dingtalk/compare/v1.2.2...v1.2.3
[1.0.0]: https://github.com/soulteary/herald-dingtalk/compare/v0.7.0...v1.0.0
