# Security Policy

## Supported versions

Security fixes are provided for the latest `v1.x` release line. The plugin protocol is pinned to `iot-rule-plugin-sdk@v1.0.0` until a deliberately reviewed compatibility release changes that contract.

## Reporting a vulnerability

Do not open a public issue for credentials, authentication bypasses, plugin execution vulnerabilities, or other exploitable findings. Use GitHub's private vulnerability reporting for this repository. Include the affected version, reproduction steps, impact, and any proposed mitigation. Do not include live credentials.

## Deployment warning

Plugin artifacts are native executables. Runtime installation is disabled by default and must not be enabled without a trusted, externally signature-verified release manifest and OS-level isolation. See `docs/security-deployment.md`.

Any NATS credential that has appeared in Git history must be revoked and rotated even if its file was later deleted.
