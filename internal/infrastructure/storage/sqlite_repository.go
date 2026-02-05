package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go-email-api/internal/domain"
)

type SQLiteRepository struct {
	dbPath string
}

func NewSQLiteRepository(dbPath string) (*SQLiteRepository, error) {
	if dbPath == "" {
		return nil, errors.New("db path is required")
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o750); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, fmt.Errorf("prepare db directory: %w", err)
	}
	repo := &SQLiteRepository{dbPath: dbPath}
	if err := repo.migrate(context.Background()); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *SQLiteRepository) Close() error { return nil }

func (r *SQLiteRepository) migrate(ctx context.Context) error {
	sql := `PRAGMA foreign_keys = ON;
CREATE TABLE IF NOT EXISTS accounts (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 name TEXT NOT NULL,
 protocol TEXT NOT NULL CHECK(protocol IN ('imap','pop3')),
 host TEXT NOT NULL,
 port INTEGER NOT NULL,
 username TEXT NOT NULL,
 password TEXT NOT NULL,
 use_tls INTEGER NOT NULL,
 mailbox TEXT NOT NULL,
 last_synced_at TEXT,
 last_synced_cursor TEXT NOT NULL DEFAULT '',
 created_at TEXT NOT NULL,
 updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS emails (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 remote_id TEXT NOT NULL,
 subject TEXT NOT NULL,
 from_address TEXT NOT NULL,
 to_address TEXT NOT NULL,
 received_at TEXT NOT NULL,
 raw_path TEXT NOT NULL,
 raw_sha256 TEXT NOT NULL,
 raw_size_bytes INTEGER NOT NULL,
 preview TEXT NOT NULL,
 stored_at TEXT NOT NULL,
 message_id TEXT NOT NULL,
 thread_id TEXT NOT NULL,
 in_reply_to TEXT NOT NULL,
 references_header TEXT NOT NULL,
 has_attachments INTEGER NOT NULL,
 UNIQUE(account_id, remote_id)
);
CREATE INDEX IF NOT EXISTS idx_emails_account_received_at ON emails(account_id, received_at DESC);`
	_, err := r.execSQL(ctx, sql)
	return err
}

func (r *SQLiteRepository) CreateAccount(ctx context.Context, account domain.Account) (domain.Account, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	q := fmt.Sprintf(`INSERT INTO accounts (name, protocol, host, port, username, password, use_tls, mailbox, created_at, updated_at)
VALUES (%s, %s, %s, %d, %s, %s, %d, %s, %s, %s);
SELECT last_insert_rowid() AS id;`,
		quote(account.Name), quote(string(account.Protocol)), quote(account.Host), account.Port, quote(account.Username), quote(account.Password), boolToInt(account.UseTLS), quote(account.Mailbox), quote(now), quote(now))
	out, err := r.execSQL(ctx, q)
	if err != nil {
		return domain.Account{}, err
	}
	id, err := parseLastInsertID(out)
	if err != nil {
		return domain.Account{}, err
	}
	account.ID = id
	account.CreatedAt, _ = time.Parse(time.RFC3339Nano, now)
	account.UpdatedAt = account.CreatedAt
	return account, nil
}

func (r *SQLiteRepository) GetAccount(ctx context.Context, id int64) (domain.Account, error) {
	q := fmt.Sprintf(`SELECT id, name, protocol, host, port, username, password, use_tls, mailbox, COALESCE(last_synced_at,'') AS last_synced_at, last_synced_cursor, created_at, updated_at FROM accounts WHERE id = %d LIMIT 1;`, id)
	rows, err := r.queryJSON(ctx, q)
	if err != nil {
		return domain.Account{}, err
	}
	if len(rows) == 0 {
		return domain.Account{}, fmt.Errorf("account %d not found", id)
	}
	return mapAccount(rows[0])
}

func (r *SQLiteRepository) ListAccounts(ctx context.Context) ([]domain.Account, error) {
	q := `SELECT id, name, protocol, host, port, username, password, use_tls, mailbox, COALESCE(last_synced_at,'') AS last_synced_at, last_synced_cursor, created_at, updated_at FROM accounts ORDER BY id ASC;`
	rows, err := r.queryJSON(ctx, q)
	if err != nil {
		return nil, err
	}
	accounts := make([]domain.Account, 0, len(rows))
	for _, row := range rows {
		acc, err := mapAccount(row)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, acc)
	}
	return accounts, nil
}

func (r *SQLiteRepository) UpdateAccountSyncCursor(ctx context.Context, accountID int64, cursor string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	q := fmt.Sprintf(`UPDATE accounts SET last_synced_cursor=%s, last_synced_at=%s, updated_at=%s WHERE id=%d;`, quote(cursor), quote(now), quote(now), accountID)
	_, err := r.execSQL(ctx, q)
	return err
}

func (r *SQLiteRepository) SaveEmailMetadata(ctx context.Context, email domain.EmailMetadata) (domain.EmailMetadata, error) {
	q := fmt.Sprintf(`INSERT OR IGNORE INTO emails (account_id, remote_id, subject, from_address, to_address, received_at, raw_path, raw_sha256, raw_size_bytes, preview, stored_at, message_id, thread_id, in_reply_to, references_header, has_attachments)
VALUES (%d, %s, %s, %s, %s, %s, %s, %s, %d, %s, %s, %s, %s, %s, %s, %d);
SELECT last_insert_rowid() AS id;`,
		email.AccountID, quote(email.RemoteID), quote(email.Subject), quote(email.FromAddress), quote(email.ToAddress), quote(email.ReceivedAt.Format(time.RFC3339Nano)), quote(email.RawPath), quote(email.RawSHA256), email.RawSizeBytes, quote(email.Preview), quote(email.StoredAt.Format(time.RFC3339Nano)), quote(email.MessageID), quote(email.ThreadID), quote(email.InReplyTo), quote(email.References), boolToInt(email.HasAttachments))
	out, err := r.execSQL(ctx, q)
	if err != nil {
		return domain.EmailMetadata{}, err
	}
	id, _ := parseLastInsertID(out)
	email.ID = id
	return email, nil
}

func (r *SQLiteRepository) ListEmailsByAccount(ctx context.Context, accountID int64, limit, offset int) ([]domain.EmailMetadata, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	q := fmt.Sprintf(`SELECT id, account_id, remote_id, subject, from_address, to_address, received_at, raw_path, raw_sha256, raw_size_bytes, preview, stored_at, message_id, thread_id, in_reply_to, references_header, has_attachments FROM emails WHERE account_id=%d ORDER BY received_at DESC LIMIT %d OFFSET %d;`, accountID, limit, offset)
	rows, err := r.queryJSON(ctx, q)
	if err != nil {
		return nil, err
	}
	result := make([]domain.EmailMetadata, 0, len(rows))
	for _, row := range rows {
		e, err := mapEmail(row)
		if err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, nil
}

func (r *SQLiteRepository) execSQL(ctx context.Context, sql string) (string, error) {
	cmd := exec.CommandContext(ctx, "sqlite3", r.dbPath, sql)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("sqlite exec failed: %w, output=%s", err, string(out))
	}
	return strings.TrimSpace(string(out)), nil
}

func (r *SQLiteRepository) queryJSON(ctx context.Context, sql string) ([]map[string]any, error) {
	cmd := exec.CommandContext(ctx, "sqlite3", "-json", r.dbPath, sql)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("sqlite query failed: %w, output=%s", err, string(out))
	}
	trim := strings.TrimSpace(string(out))
	if trim == "" {
		return []map[string]any{}, nil
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(trim), &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func parseLastInsertID(out string) (int64, error) {
	v := strings.TrimSpace(out)
	if v == "" {
		return 0, nil
	}
	parts := strings.Split(v, "\n")
	v = parts[len(parts)-1]
	return strconv.ParseInt(v, 10, 64)
}

func quote(v string) string {
	return "'" + strings.ReplaceAll(v, "'", "''") + "'"
}

func mapAccount(row map[string]any) (domain.Account, error) {
	acc := domain.Account{}
	acc.ID = int64(row["id"].(float64))
	acc.Name = toString(row["name"])
	acc.Protocol = domain.Protocol(toString(row["protocol"]))
	acc.Host = toString(row["host"])
	acc.Port = int(row["port"].(float64))
	acc.Username = toString(row["username"])
	acc.Password = toString(row["password"])
	acc.UseTLS = int(row["use_tls"].(float64)) == 1
	acc.Mailbox = toString(row["mailbox"])
	acc.LastSyncedCursor = toString(row["last_synced_cursor"])
	acc.CreatedAt, _ = time.Parse(time.RFC3339Nano, toString(row["created_at"]))
	acc.UpdatedAt, _ = time.Parse(time.RFC3339Nano, toString(row["updated_at"]))
	if ls := toString(row["last_synced_at"]); ls != "" {
		t, _ := time.Parse(time.RFC3339Nano, ls)
		acc.LastSyncedAt = &t
	}
	return acc, nil
}

func mapEmail(row map[string]any) (domain.EmailMetadata, error) {
	e := domain.EmailMetadata{}
	e.ID = int64(row["id"].(float64))
	e.AccountID = int64(row["account_id"].(float64))
	e.RemoteID = toString(row["remote_id"])
	e.Subject = toString(row["subject"])
	e.FromAddress = toString(row["from_address"])
	e.ToAddress = toString(row["to_address"])
	e.ReceivedAt, _ = time.Parse(time.RFC3339Nano, toString(row["received_at"]))
	e.RawPath = toString(row["raw_path"])
	e.RawSHA256 = toString(row["raw_sha256"])
	e.RawSizeBytes = int64(row["raw_size_bytes"].(float64))
	e.Preview = toString(row["preview"])
	e.StoredAt, _ = time.Parse(time.RFC3339Nano, toString(row["stored_at"]))
	e.MessageID = toString(row["message_id"])
	e.ThreadID = toString(row["thread_id"])
	e.InReplyTo = toString(row["in_reply_to"])
	e.References = toString(row["references_header"])
	e.HasAttachments = int(row["has_attachments"].(float64)) == 1
	return e, nil
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
