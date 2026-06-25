## Summary

<!-- What this PR does and why. -->

## Type of change

- [ ] feat — new feature
- [ ] fix — bug fix
- [ ] docs — documentation
- [ ] style — formatting, no logic change
- [ ] refactor — refactoring without behavior change
- [ ] perf — performance improvement
- [ ] test — tests
- [ ] build — build/dependencies
- [ ] ci — continuous integration
- [ ] chore — general maintenance
- [ ] revert — revert a previous commit

## Checklist

- [ ] Commits follow Conventional Commits with a required scope (`type(scope): description`)
- [ ] `go build -o gitmera .` passes locally
- [ ] `go test ./... -race` passes locally
- [ ] Total test coverage remains above 80% (`go test ./... -coverprofile=c.out && go tool cover -func=c.out | grep total`)
- [ ] `golangci-lint run` passes locally (when changing `.go`/`go.mod`/`go.sum`)
- [ ] New/updated tests covering the change, where applicable

## How to test

<!-- Steps to validate manually, if applicable. -->

## Related issues

<!-- Closes #123 -->
