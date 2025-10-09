# go-eloquent

Lightweight Eloquent-like MongoDB ORM and helpers in Go.

## Quick start

- Run tests: `go test ./...`
- Build: `go build ./...`

## Development

Install dev tooling:

- Install golangci-lint (recommended):
  - `go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.59.0`
  - Then run `golangci-lint run ./...`

- Run vet: `go vet ./...`
- Run tests: `go test ./... -v`

## CI

A GitHub Actions workflow is provided at `.github/workflows/ci.yml` which runs `go vet`, `go test` and `golangci-lint` on push/PR to `main`.

## Notes

- Canonical string utilities live in `libs/strings` (package `strlib`).
- Compatibility shims under `src/Libs/Strings` were removed — update external imports to `github.com/venomous-maker/go-eloquent/libs/strings` if needed.

## Contributing

See CONTRIBUTING.md for contributor guidelines.

