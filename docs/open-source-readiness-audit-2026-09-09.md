# IoT Rule Engine public-release readiness audit

- Audit date: 2026-09-09
- Scope: engine backend, management UI, deployment templates, documentation, and repository governance
- Release verdict: **not ready to publish yet**; runtime quality is strong, but repository hygiene and
  release engineering remain blockers

## Executive assessment

The engine has broad automated coverage, race testing, process-isolated plugins, bounded dispatch paths,
embedded NATS defaults, and a usable management UI. The highest public-release risks are not the core
runtime: they are inherited credential history, environment-specific documentation, AGPL compliance
guidance, and the absence of a finalized reproducible release pipeline.

## Findings

| ID | Severity | Finding | Required action before GitHub publication |
| --- | --- | --- | --- |
| ENG-OS-001 | Resolved | The license decision was finalized on 2026-09-09. | `LICENSE` and both README languages now identify `AGPL-3.0-only`; verify copyright ownership and publish an AGPL compliance guide before the first public tag. |
| ENG-OS-002 | P0 | Previously tracked `.creds` files are deleted in the working tree but may remain recoverable from Git history. | Rotate/revoke every historical credential, scan all commits, and either publish a clean history or use a reviewed history-rewrite procedure. |
| ENG-OS-003 | P1 | Repository remotes still point to a local legacy clone rather than the future GitHub repository. | Create the GitHub repository, configure protected branches and environments, then replace remotes only after local release gates pass. |
| ENG-OS-004 | P1 | Site-specific deployment IPs remain in five files. No known MQTT username/password literal was found. | Replace addresses with RFC 5737 documentation ranges or clearly parameterized examples. |
| ENG-OS-005 | P1 | Site-specific rule JSON and integration artifacts are present or staged under `docs/`. | Sanitize and move reusable samples to `examples/`; exclude customer/device exports from the public repository. |
| ENG-OS-006 | P1 | Startup still requires `configs/config.yaml`, although runtime settings are persisted in SQLite. | Keep documentation accurate now; implement and test the versioned single-binary bootstrap plan before claiming file-independent startup. |
| ENG-OS-007 | P1 | CI tests Go, race, vet, UI tests, and UI build, but lacks dependency review, secret scanning, CodeQL, release signing, and an engine SBOM. | Add least-privilege security workflows and a signed, reproducible release workflow. |
| ENG-OS-008 | P2 | The engine CI checks out SDK tag `v1.0.0`; the future public tag and repository must exist before CI can pass. | Publish and protect the SDK tag before enabling engine CI. |
| ENG-OS-009 | P2 | The working tree contains a large set of intentional but uncommitted migrations. | Split changes into reviewable commits and preserve provenance; do not upload a partially staged tree. |

## Verified strengths

- Dispatcher, plugin, connector, runtime, storage, Manager, and UI tests are present.
- CI uses read-only GitHub token permissions and runs the race detector.
- Plugin processes are isolated, while instances remain independent per rule node.
- Event clone/ownership rules prevent shared-map crashes across concurrent branches.
- Initial administrator plaintext is not stored: the environment value is validated and converted to a
  bcrypt hash before insertion into `sys_users`.
- Known NATS credential files are ignored and currently marked for deletion from the working tree.
- The refreshed visual identity no longer uses the legacy project initials.

## Required release gates

1. Verify copyright ownership and third-party license compatibility; publish AGPL network-use compliance and trademark guidance.
2. Rotate historical credentials and run a full-history secret scan.
3. Sanitize deployment endpoints, rule exports, screenshots, and device examples.
4. Publish SDK `v1.0.0`, then validate engine and plugin CI against the public tag.
5. Add dependency review, CodeQL, secret scanning, SBOM generation, checksums, and signed release artifacts.
6. Run unit, race, vet, UI, migration, empty-database bootstrap, and upgrade/rollback tests from a clean clone.
7. Create reviewable commits and a signed release candidate; only then create or push the GitHub repository.

## Verification snapshot

The 2026-09-09 working tree passed `go test ./...`, `go vet ./...`, all 58 UI tests, and the optimized UI
production build. The rule-list interaction suite was also hardened so Ant Design cleanup latency in CI
does not turn correct deploy/undeploy behavior into a five-second test timeout. These results validate the
current tree only; the release gates above still require a clean-clone run from the final release commit.

## License implication

The repository now uses `AGPL-3.0-only`, an OSI-approved copyleft license. It permits commercial use and
therefore does not enforce the previously considered "free self-use but paid commercial use" policy.
Modified versions operated for remote users require careful review of AGPL section 13 and Corresponding
Source delivery. Project names and visual identity should be governed separately through trademark policy.
