# GORM is the exclusive persistence adapter in hexagonal architecture

Status: accepted, 2026-10-27. Amends ADR-0005 and implements the hexagonal persistence adapter for the package-by-feature clean architecture.

## Context

The repository was restructured from a layered, fragmented architecture (`internal/store`, `internal/change`, `internal/api`) into a hexagonal (ports and adapters) architecture organized by domain feature (`internal/entry`, `internal/project`, `internal/team`, `internal/user`, `internal/report`, `internal/audit`, `internal/onboarding`). During this transition, we evaluated whether to replace GORM with handwritten SQL via `sqlc` or retain GORM.

## Decision

Persistence is encapsulated strictly behind domain repository interfaces (`entry.Repository`, `project.Repository`, etc.) and implemented exclusively using GORM over `glebarez/sqlite` (pure-Go SQLite, no CGO) in `internal/repository/gorm/`. All runtime raw SQL (`db.Raw`, `db.Exec`) in queries is banned across the codebase. SQL is explicitly permitted for database migrations, schema definitions, and database triggers.

## Why

- **Hexagonal Boundary Protection**: Domain feature packages define their own pure domain models and minimal repository interfaces. Domain services have zero dependencies on `gorm.io/gorm`, making domain logic testable without database mocks.
- **Capitalizing on Existing Storage Foundations**: GORM's schema engine and query builder already power the 16 tables, multi-workspace scoping callbacks (`internal/repository/gorm/tenancy.go`), and the complex transactional storage state machines for Ory Fosite OAuth 2.1 (`oauth_store.go`).
- **Eliminating SQL Injection & Scoping Bypasses**: Banning runtime raw SQL strings prevents subtle query syntax discrepancies, SQL injection hazards, and bypasses of GORM's automatic workspace isolation callbacks.
- **Reliable Audit Immutability**: Permitting SQL in migrations allows tamper-evident database triggers (`BEFORE UPDATE` and `BEFORE DELETE` on `audit_log` with `RAISE(ABORT)`) and SQLite partial indexes (such as `idx_entries_one_running`) that cannot be fully expressed in ORM struct tags alone.
- **Context-Bound Unit of Work**: Transactions are coordinated across multiple repository mutations via a `UnitOfWork` runner (`tx.Run`) that manages GORM transactions and propagates the transaction context transparently without leaking ORM handles to services.

## Considered Options

- **Switching to `sqlc` with handwritten SQL migrations**: Rejected because rewriting Fosite's complex OAuth 2.1 state machine (single-use code replay revocation, token rotation families, PKCE verification) and porting dynamic tenancy callbacks to static SQL adds high regression risk without functional gain.
- **Allowing hybrid raw SQL queries alongside GORM**: Rejected because raw SQL queries bypass GORM tenancy callbacks and create dual maintenance paths.
- **Application-only audit immutability (GORM hooks only)**: Rejected because database-level SQLite triggers guarantee that even direct SQL access or operator scripts cannot mutate or delete audit entries.

## Consequences

- Domain feature packages define their own models and repository interfaces; GORM models and tags are strictly confined to `internal/repository/gorm/`.
- Runtime query code must use GORM's query builder methods (`Where`, `Select`, `Joins`, `First`, `Find`, `Create`, `Updates`, `Delete`) and clauses exclusively.
- Schema migrations continue to use GORM `AutoMigrate` for table structures, supplemented by embedded SQL migration files (`migrations/*.sql`) for database triggers and indexes.
- The two-binary build pipeline (ADR-0008) is preserved: `cmd/tstamps` remains a lightweight pure HTTP client CLI, and `cmd/tstamps-server` owns the database and runs the server daemon.
