# Go Email API

A clean-architecture Go email ingestion service that:

- Connects to remote email providers over **IMAP** or **POP3**.
- Downloads raw MIME messages and stores them securely on local disk.
- Persists mailbox/account metadata and searchable email metadata in **SQLite**.

## Features

- Pure domain/application boundaries using ports/adapters.
- SQLite repository adapter with schema migration and JSON-based querying via the local `sqlite3` binary.
- Raw message storage with path sanitization, SHA-256 checksums, and restrictive file permissions.
- HTTP API to register accounts, trigger sync, and list emails.
- Defensive server configuration and basic security headers.

## Project layout

- `cmd/server`: app bootstrap.
- `internal/domain`: entities and interfaces (ports).
- `internal/application`: business use-cases.
- `internal/infrastructure/storage`: SQLite + disk adapters.
- `internal/infrastructure/mail`: IMAP/POP3 adapters.
- `internal/infrastructure/api`: HTTP transport adapter.

## API

### Register account

```bash
curl -X POST http://localhost:8080/accounts \
  -H 'content-type: application/json' \
  -d '{
    "name":"work-imap",
    "protocol":"imap",
    "host":"imap.example.com",
    "port":993,
    "username":"alice@example.com",
    "password":"strong-password",
    "use_tls":true,
    "mailbox":"INBOX"
  }'
```

### Sync account

```bash
curl -X POST http://localhost:8080/accounts/1/sync
```

### List stored emails

```bash
curl "http://localhost:8080/accounts/1/emails?limit=50&offset=0"
```

## Run

```bash
go run ./cmd/server
```

Environment variables:

- `APP_ADDR` (default `:8080`)
- `APP_DB_PATH` (default `./data/email.db`)
- `APP_RAW_DIR` (default `./data/raw`)

## Security notes

- Store credentials securely (prefer secret managers in production).
- The API currently stores account passwords encrypted-at-rest only if the underlying filesystem/db provides it; add dedicated application-level encryption for strict compliance.
- Expose this API behind TLS and authentication/authorization in production.
