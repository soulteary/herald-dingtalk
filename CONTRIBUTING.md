# Contributing to herald-dingtalk

Thank you for your interest in contributing to herald-dingtalk.

## Development

- **Go version**: 1.26+ (see [go.mod](go.mod)).
- **Tests**: Run `go test -count=1 ./...`; use `go test -race ./...` for concurrency checks. Generate coverage with `go test -coverprofile=coverage.out ./...`, then inspect it with `go tool cover -func=coverage.out` or `go tool cover -html=coverage.out`. CI requires total statement coverage of at least 90%. Mobile lookup behavior is exercised automatically without environment-specific test commands.
- **Code style**: Follow standard Go formatting. Run `gofmt -s -w .` before committing. The CI runs `gofmt -s -l .` and fails if there are unformatted files.
- **Static analysis**: CI runs `go vet ./...`. Run `golangci-lint run` locally (e.g. errcheck) before submitting.

## Submitting changes

1. Fork the repository and create a branch from `main` (or `master`).
2. Make your changes; keep commits focused and messages clear.
3. Ensure tests and race detection pass: `go test -count=1 ./...` and `go test -race ./...`.
4. Open a Pull Request with a short description of the change and reference any related issues.

## Documentation

- English docs: [docs/enUS/](docs/enUS/).
- Chinese docs: [docs/zhCN/](docs/zhCN/).
- When adding or changing API or configuration, update the relevant docs (API.md, DEPLOYMENT.md, README) in both enUS and zhCN if applicable.

## Questions

Open an [Issue](https://github.com/soulteary/herald-dingtalk/issues) for questions or bug reports.
