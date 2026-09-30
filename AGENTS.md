# AGENTS.md

This file is operational guidance for AI coding agents working in `iot-rule-engine`.

## Repository role

This repository owns the Go engine, Manager API, ingress services, rule scheduler/runtime, SQLite and
PostgreSQL control-plane adapters, and the React UI. Production plugin implementations belong in the
sibling `iot-rule-plugins` repository. Public plugin contracts belong in `iot-rule-plugin-sdk`.

## Read before changing code

- Inspect the working tree first. It may contain intentional user changes; never reset or discard them.
- Read the nearest design document under `docs/` for the subsystem being changed.
- When changing SDK-facing behavior, also inspect sibling SDK v1 contracts and compatibility policy.
- Do not commit, tag, push, publish releases, or change license terms unless the user explicitly asks.
- The repository is `AGPL-3.0-only`. Preserve license notices and flag dependency or distribution changes
  that could affect AGPL compliance; project trademarks are not granted by the software license.

## Architecture invariants

- Every scheduled branch owns an independent `sdk.Event`. Clone at the plugin boundary and for every
  downstream branch. Never share mutable `Payload` or `Metadata` maps across concurrent tasks.
- Scheduling data such as source node IDs and start times belongs in dispatch/task context, not event
  metadata. Observers receive immutable value contexts containing only required fields.
- One plugin artifact starts at most one OS process. Each rule node gets an independent in-process plugin
  instance through `ServeFactory`; stopping one rule must not stop other instances or the shared process.
- Connector resources are configuration/state holders. Physical sessions follow protocol demand. S7 uses
  scheduled reuse, serial device access, latest-only scans, bounded reconnects, and immediate close on
  broken/no-demand/stop boundaries. MQTT may retain a persistent connection.
- Embedded NATS is the default (`internal`, loopback, port `0`). Never add repository credentials for it.
- Manager API defaults to port `11011`. Development UI defaults to that address; production UI uses
  same-origin `/api/v1`. The port is a bootstrap setting, not an online-editable field.
- SQLite configuration is authoritative after initial seeding, but the current release still requires a
  YAML bootstrap file. Do not claim otherwise until the single-binary migration is implemented.
- Initial administrator plaintext comes only from `IOT_RULE_ADMIN_PASSWORD`; only its bcrypt hash is stored.

## Code map

- `cmd/`: process entry point.
- `internal/core/`: engine, dispatcher, scheduler, plugin, connector, and runtime domain logic.
- `internal/infra/`: bus, configuration, pipeline, and persistence adapters.
- `internal/server/`: Manager, ingress, executor, and composition root.
- `ui/`: React/TypeScript management console.
- `configs/`: bootstrap configuration templates only; no secrets.
- `docs/`: architecture, audit, deployment, and operational documentation.
- `deploy/`: packaging and deployment templates.

## Change rules

- Prefer explicit ownership, bounded queues, deadlines, and cancellation over shared mutable state.
- Go fatal concurrent-map errors cannot be recovered; prevent aliasing instead of adding panic recovery.
- Validate destructive plugin operations against deployed-rule dependencies.
- Runtime edge links must only be rendered when a retrievable edge detail resource exists.
- Keep UI API calls centralized in `ui/src/services/api.ts` and preserve same-origin production behavior.
- Keep English as the default repository README and maintain `README.zh-CN.md` for Chinese readers.
- Never include live IPs, credentials, certificates, customer names, PLC addresses, or production payloads
  in examples. Use reserved documentation networks and synthetic data.

## Verification

For Go changes:

```bash
go test ./...
go test -race ./...
go vet ./...
```

For UI changes:

```bash
cd ui
CI=true npm test -- --runInBand
npm run build
```

Run focused tests first, then the full applicable suite. Use `git diff --check` before handoff. Do not start
real device or broker integrations during tests without explicit authorization.

## Generated and runtime artifacts

Do not commit `ui/node_modules`, `ui/build`, databases, logs, NATS state, plugin binaries, temporary
certificates, `.creds` files, or deployment `.env` files. Treat any deleted credential file in Git history
as potentially compromised until rotation and history review are complete.
