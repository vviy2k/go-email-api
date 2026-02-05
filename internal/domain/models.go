package domain

import "time"

type Protocol string

const (
	ProtocolIMAP Protocol = "imap"
	ProtocolPOP3 Protocol = "pop3"
)

type Account struct {
	ID               int64
	Name             string
	Protocol         Protocol
	Host             string
	Port             int
	Username         string
	Password         string
	UseTLS           bool
	Mailbox          string
	LastSyncedAt     *time.Time
	LastSyncedCursor string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type EmailMetadata struct {
	ID             int64     `json:"id"`
	AccountID      int64     `json:"account_id"`
	RemoteID       string    `json:"remote_id"`
	Subject        string    `json:"subject"`
	FromAddress    string    `json:"from_address"`
	ToAddress      string    `json:"to_address"`
	ReceivedAt     time.Time `json:"received_at"`
	RawPath        string    `json:"raw_path"`
	RawSHA256      string    `json:"raw_sha256"`
	RawSizeBytes   int64     `json:"raw_size_bytes"`
	Preview        string    `json:"preview"`
	StoredAt       time.Time `json:"stored_at"`
	MessageID      string    `json:"message_id"`
	ThreadID       string    `json:"thread_id"`
	InReplyTo      string    `json:"in_reply_to"`
	References     string    `json:"references"`
	HasAttachments bool      `json:"has_attachments"`
}

type RemoteEmail struct {
	RemoteID       string
	MessageID      string
	ThreadID       string
	InReplyTo      string
	References     string
	Subject        string
	FromAddress    string
	ToAddress      string
	ReceivedAt     time.Time
	Preview        string
	RawMIME        []byte
	HasAttachments bool
	Cursor         string
}
