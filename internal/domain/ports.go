package domain

import "context"

type EmailRepository interface {
	CreateAccount(ctx context.Context, account Account) (Account, error)
	GetAccount(ctx context.Context, id int64) (Account, error)
	ListAccounts(ctx context.Context) ([]Account, error)
	UpdateAccountSyncCursor(ctx context.Context, accountID int64, cursor string) error
	SaveEmailMetadata(ctx context.Context, email EmailMetadata) (EmailMetadata, error)
	ListEmailsByAccount(ctx context.Context, accountID int64, limit, offset int) ([]EmailMetadata, error)
}

type RawEmailStore interface {
	Store(ctx context.Context, accountID int64, remoteID string, raw []byte) (path, sha256sum string, size int64, err error)
}

type EmailClient interface {
	Fetch(ctx context.Context, account Account, cursor string) ([]RemoteEmail, string, error)
}
