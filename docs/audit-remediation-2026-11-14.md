# 2026-11-14 audit remediation

## Implemented and verified

- P1-01: serialize resource lifecycle changes; reject connection changes while bound;
  retire idle provider sessions on connection/session/provider changes.
- P1-02: reject registry scheme changes, including same-host HTTPS downgrade.
- P1-03: authenticated encryption for SQLite Connector records, protected external key,
  transactional legacy migration, missing/wrong-key fail-closed behavior.
- P2-01: apply Connector changes before persistence; attempt runtime rollback on database
  failure and report rollback failure explicitly.
- Hub: named administrator credentials, actor in publication/revocation history, structured
  success/failure request logs, file synchronization before catalog rename.

Engine full tests, race tests and vet passed. Targeted tests cover idle provider recreation,
bound resource rejection, encrypted storage/reopen, tampering, legacy migration and missing key.
Hub full race tests and vet passed. No SDK public contract was changed.

## Remaining acceptance work

This is not closure of every finding. MQTT business-commit acknowledgements, persistent
subscriber recovery, disk-full/long-duration testing, PostgreSQL secret protection, granular
Hub roles and audit retention, browser visual review, dependency refresh and actual ARM/PLC/
database integration still require implementation or acceptance. No end-to-end exactly-once
or loss-free delivery claim is made. Existing Broker acknowledgements retain their documented
meaning. FANUC, Mitsubishi and Fleet remain planned capabilities.

Configuration apply/persist spans separate systems: rollback is best effort, not a distributed
transaction. Multi-manager concurrent editing and applied-generation reconciliation remain
follow-up work. See security-deployment.md before upgrading an existing SQLite instance.
