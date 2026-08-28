# Issue #69 — Transaction Screening, OFAC Sanctions & Suspicious Activity Flagging

Branch: `feat/issue-69-compliance-screening`

## Phase 0 — Unblock: repair broken `main` (pre-existing, not part of #69)

`main` did not compile. Root cause: bad merge resolution in `8c6e601`
("Merge remote-tracking branch 'upstream/main' into fix/issue-85-86…") plus
follow-on mock drift. Fixed on this branch so the plan's verification
(`go build`, `make test`, `make lint`) can actually run.

- [x] `internal/domain/wallet.go` — `SyncCursor` declared twice
- [x] `internal/domain/transaction.go` — restore 5 fiat fields the repo layer reads/writes
- [x] `internal/stellar/client.go` — drop dead `PaymentsForAccount` (removed Horizon API)
- [x] `internal/fiat/service.go` — `evt.Reference` → `evt.ProviderRef`
- [x] `internal/fiat/flutterwave/provider.go` — missing `decimal` import
- [x] `internal/postgres/transaction_repo.go` — unused `localAmt`
- [x] `internal/anchor/repository.go` — tenant-scoped signatures to match impl
- [x] `cmd/api/main.go` — restore Flutterwave rail via `NewRailAdapter` (user decision)
- [x] Test mock drift: transfer ×2, batch, indexer, schedule, org
- [x] `go build ./...`, `go vet ./...`, `go test ./... -race` all green (19 pkgs)

## Phase 1 — Schema

- [ ] `000022_add_compliance_hold_status.{up,down}.sql` (one-line ALTER TYPE, alone)
- [ ] `000023_create_compliance_tables.{up,down}.sql` (4 tables + hot-path indexes)

## Phase 2 — Domain

- [ ] `domain.StatusComplianceHold`
- [ ] `domain/compliance.go` — review/block/sanctions types + screening request/decision
- [ ] 3 sentinel errors + `HandleDomainError` arms (403 `TRANSFER_BLOCKED_SANCTIONS`)
- [ ] 4 webhook event types

## Phase 3 — `internal/compliance/`

- [ ] `screener.go` — interface, composite, precedence blocked > hold > clear
- [ ] `levenshtein.go` — distance-capped edit distance
- [ ] `sanctions.go` — `SanctionsSet` + `SanctionsScreener`
- [ ] `sdn.go` — `SDNSource` + streaming XML parser
- [ ] `velocity.go` — velocity / structuring / round-trip
- [ ] `repository.go`, `service.go`, `handler.go`, `worker.go`
- [ ] `testdata/sdn_sample.xml`

## Phase 4 — Integration

- [ ] `internal/postgres/compliance_repo.go` (tenant-scoped)
- [ ] `transfer/service.go` — screen in `initiate()`, `WithScreener` builder
- [ ] `queue` task type + enqueue helper
- [ ] `server.go` mount at `/admin/compliance`
- [ ] `cmd/api` + `cmd/worker` wiring
- [ ] config + `.env.example`
- [ ] batch `aggregateStatus` — explicit `compliance_hold` arm

## Phase 5 — Tests (acceptance criteria)

- [ ] Sanctioned address → 403, zero enqueues
- [ ] 3×999 holds, 3×1000 does not
- [ ] SDN refresh parses + records update row
- [ ] Approve resets to `pending` AND enqueues
- [ ] Hold does not block the org's other transfers
- [ ] Fuzzy federation match → hold, not blocked
- [ ] Composite precedence, fail-closed, refresh_failed webhook
- [ ] `compliance_hold` invisible to reconciliation

## Phase 6 — Docs

- [ ] `docs/errors.md`, `docs/openapi.yaml`, `README.md`, `ASSUMPTIONS.md`
