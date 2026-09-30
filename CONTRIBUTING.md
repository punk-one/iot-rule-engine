# Contributing

## Development contract

- Use Go 1.25 or the version declared in `go.mod`.
- Use `iot-rule-plugin-sdk@v1.0.0`; do not add local `replace` directives to committed modules.
- Keep protocol/database drivers in independent plugin repositories. Engine code may share only the SDK contract with plugins.
- Treat Event as owned mutable state: clone at external boundaries and once per asynchronous branch. Keep scheduling fields out of Metadata.
- Every long-lived NATS subscription must have an owner and be closed during that owner's shutdown.
- Never commit passwords, tokens, creds, private keys, generated plugin binaries, or deployment data.

## Required checks

```bash
gofmt -w <changed-go-files>
go test ./...
go test -race ./...
go vet ./...
git diff --check
```

Changes to plugin loading, scheduling, event cloning, service supervision, authentication, or NATS lifecycle require focused regression tests in addition to the full suite.
