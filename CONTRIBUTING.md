# Contributing to NetBerth

## License & CLA

NetBerth is dual-licensed: AGPL-3.0 for the open-source core, with a commercial license for the enterprise module. By submitting a pull request, you agree to license your contribution under AGPL-3.0.

## Development Setup

```bash
git clone https://github.com/netberth/netberth.git
cd netberth
go mod download
go build ./...
go test -short ./...
```

Frontend: `cd web && npm install && npm run build`

## Pull Request Process

1. Fork and branch from `main`
2. Write tests for new functionality
3. Ensure `go vet ./...` and `go test -short ./...` pass
4. Run `go test -count=3 -short ./...` for stable CI
5. Open PR with description of changes

## Code Style

- Go: standard formatting (`go fmt`). No panic in library code.
- TypeScript: strict mode. shadcn/ui components.
- Tests: use `testing.T` helpers. Prefer `osPort(t)` for network tests.
- Security: validate all inputs at boundaries. No hardcoded secrets.

## Security

Report vulnerabilities via GitHub Security Advisories, not public issues. See SECURITY.md.
