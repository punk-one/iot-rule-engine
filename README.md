# IoT Rule Engine

[English](README.md) | [简体中文](README.zh-CN.md)

IoT Rule Engine is an edge-oriented rule orchestration runtime written in Go. It connects industrial
data sources, isolated plugins, routing rules, runtime observability, and a web management console in
one deployable control plane.

> Project status: active stabilization. The engine is currently versioned as `v1.0.0`; the plugin SDK
> and official plugins target `v1.0.0`.

## Highlights

- Visual DAG-based rule authoring and runtime inspection.
- HTTP and MQTT ingress with an embedded NATS/JetStream bus by default.
- Source, processor, sink, and connector plugins isolated in operating-system processes.
- SQLite control-plane storage by default, with PostgreSQL and Redis integrations where configured.
- Connector-aware plugins for S7, MQTT, Redis, PostgreSQL, TimescaleDB, and TDengine.
- Per-branch event ownership to prevent shared mutable payload and metadata races.
- A React management console for rules, connectors, plugins, users, settings, and runtime telemetry.

## Repository family

| Repository | Responsibility |
| --- | --- |
| `github.com/punk-one/iot-rule-engine` | Engine runtime, Manager API, ingress, scheduler, and web UI |
| `github.com/punk-one/iot-rule-plugin-sdk` | Stable v1 plugin and connector contracts |
| `github.com/punk-one/iot-rule-plugins` | Independently built official plugin artifacts |

The engine repository does not contain production plugin implementations. Test fixtures may exist under
internal test directories, while released plugins are maintained by the plugin repository.

## Runtime model

```text
HTTP / MQTT
     |
  Ingress
     |
Embedded or external NATS
     |
  Pipeline ----> rule.input.<rule-id>
     |
  Executor ----> isolated plugin processes
     |
 Runtime metrics / traces / DLQ

Manager API <---- RulesDB / users / connectors / configuration
```

One plugin artifact maps to one long-lived OS process. Every rule node using that artifact receives its
own plugin object, configuration, runtime context, and stop lifecycle inside the process. Starting more
rules does not start duplicate processes for the same artifact.

## Quick start

Requirements:

- Go 1.25 or newer.
- Node.js 22 and npm for UI development.
- Built official plugins when rules depend on external plugin nodes.

The current release still uses `configs/config.yaml` as its bootstrap template. On the first start,
provide a strong administrator password through a deployment secret:

```bash
export IOT_RULE_ADMIN_PASSWORD='Replace-With-A-Strong1!'
go run ./cmd/main.go
```

The plaintext environment value is read only when the initial `admin` user is absent. The engine stores
only a bcrypt hash in the control-plane database. Remove the secret from the long-running environment
after successful initialization when your deployment platform allows it.

Start the development UI separately:

```bash
cd ui
npm ci
npm start
```

Default endpoints:

| Service | Address |
| --- | --- |
| Manager API | `http://localhost:11011/api/v1` |
| Web UI (development) | `http://localhost:3000` |
| HTTP ingress | `http://localhost:8087` |
| MQTT ingress | `localhost:1883` |

The development UI connects to `http://localhost:11011/api/v1` by default. Set
`REACT_APP_API_URL` when the Manager is hosted elsewhere. Production builds use same-origin `/api/v1`.
The bootstrap template explicitly permits the local development UI origins; replace them with the real
HTTPS console origin in production. Same-origin requests are accepted without an allow-list entry.

## Configuration

Configuration is loaded in this order:

1. Bootstrap values from `configs/config.yaml`.
2. Persisted system configuration from the control-plane database.
3. Deployment secrets from `IOT_RULE_*` environment variables.

Embedded NATS is the default. It binds to loopback and uses port `0`, allowing the operating system to
select an available port. External NATS credentials must be injected through deployment secrets and
must never be committed.

The planned file-independent bootstrap and single-binary design is documented in
[`docs/single-binary-bootstrap-plan-2026-09-09.md`](docs/single-binary-bootstrap-plan-2026-09-09.md).

## Development checks

```bash
go test ./...
go test -race ./...
go vet ./...

cd ui
CI=true npm test -- --runInBand
npm run build
```

Do not run integration rules against real PLCs, brokers, or databases unless the target environment is
explicitly authorized.

## Documentation

- [Architecture and system overview](docs/架构与系统总览.md)
- [Runtime and scheduling](docs/运行时与调度.md)
- [Plugin and SDK rules](docs/插件与SDK规范.md)
- [Security deployment](docs/security-deployment.md)
- [Open-source readiness audit](docs/open-source-readiness-audit-2026-09-09.md)

## Security

Report vulnerabilities using the repository security policy once the public GitHub repository is
enabled. Never open a public issue containing credentials, private endpoints, device addresses, or
production rule data.

## License

IoT Rule Engine is licensed under the [GNU Affero General Public License v3.0 only](LICENSE)
(`AGPL-3.0-only`). If you modify the software and let users interact with that modified version over a
network, review section 13's Corresponding Source obligations before deployment. The AGPL permits
commercial use; it does not grant rights to project names, logos, or other trademarks.
