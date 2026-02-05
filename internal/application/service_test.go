package application

import (
	"context"
	"testing"
	"time"

	"go-email-api/internal/domain"
)

type fakeRepo struct{}

func (f fakeRepo) CreateAccount(_ context.Context, account domain.Account) (domain.Account, error) {
	account.ID = 1
	return account, nil
}
func (f fakeRepo) GetAccount(_ context.Context, _ int64) (domain.Account, error) {
	return domain.Account{}, nil
}
func (f fakeRepo) ListAccounts(_ context.Context) ([]domain.Account, error) { return nil, nil }
func (f fakeRepo) UpdateAccountSyncCursor(_ context.Context, _ int64, _ string) error {
	return nil
}
func (f fakeRepo) SaveEmailMetadata(_ context.Context, email domain.EmailMetadata) (domain.EmailMetadata, error) {
	return email, nil
}
func (f fakeRepo) ListEmailsByAccount(_ context.Context, _ int64, _, _ int) ([]domain.EmailMetadata, error) {
	return nil, nil
}

type fakeStore struct{}

func (f fakeStore) Store(_ context.Context, _ int64, _ string, raw []byte) (string, string, int64, error) {
	return "/tmp/raw", "abc", int64(len(raw)), nil
}

type fakeClient struct{}

func (f fakeClient) Fetch(_ context.Context, _ domain.Account, _ string) ([]domain.RemoteEmail, string, error) {
	return nil, "", nil
}

func TestValidateAccount(t *testing.T) {
	valid := domain.Account{
		Name:     "A",
		Protocol: domain.ProtocolIMAP,
		Host:     "imap.test",
		Port:     993,
		Username: "user",
		Password: "pwd",
	}
	if err := validateAccount(valid); err != nil {
		t.Fatalf("expected valid account, got %v", err)
	}

	invalid := valid
	invalid.Protocol = "smtp"
	if err := validateAccount(invalid); err == nil {
		t.Fatal("expected protocol validation error")
	}
}

func TestNormalizeZeroTime(t *testing.T) {
	fallback := time.Date(2026, 1, 1, 1, 1, 1, 0, time.UTC)
	if got := normalizeZeroTime(time.Time{}, fallback); !got.Equal(fallback) {
		t.Fatalf("expected fallback time, got %v", got)
	}
}

func TestRegisterAccountDefaultsMailbox(t *testing.T) {
	svc := NewService(fakeRepo{}, fakeStore{}, Clients{IMAP: fakeClient{}, POP3: fakeClient{}})
	acc, err := svc.RegisterAccount(context.Background(), domain.Account{
		Name:     "A",
		Protocol: domain.ProtocolPOP3,
		Host:     "pop.test",
		Port:     995,
		Username: "user",
		Password: "pwd",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acc.Mailbox != "INBOX" {
		t.Fatalf("expected INBOX default, got %s", acc.Mailbox)
	}
}
