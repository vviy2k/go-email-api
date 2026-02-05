package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go-email-api/internal/domain"
)

var (
	ErrInvalidProtocol = errors.New("protocol must be imap or pop3")
)

type Clients struct {
	IMAP domain.EmailClient
	POP3 domain.EmailClient
}

type Service struct {
	repo   domain.EmailRepository
	store  domain.RawEmailStore
	client Clients
	now    func() time.Time
}

func NewService(repo domain.EmailRepository, store domain.RawEmailStore, client Clients) *Service {
	return &Service{repo: repo, store: store, client: client, now: time.Now}
}

func (s *Service) RegisterAccount(ctx context.Context, account domain.Account) (domain.Account, error) {
	if err := validateAccount(account); err != nil {
		return domain.Account{}, err
	}
	account.Name = strings.TrimSpace(account.Name)
	account.Mailbox = strings.TrimSpace(account.Mailbox)
	if account.Mailbox == "" {
		account.Mailbox = "INBOX"
	}
	return s.repo.CreateAccount(ctx, account)
}

func (s *Service) ListAccounts(ctx context.Context) ([]domain.Account, error) {
	return s.repo.ListAccounts(ctx)
}

func (s *Service) ListEmails(ctx context.Context, accountID int64, limit, offset int) ([]domain.EmailMetadata, error) {
	return s.repo.ListEmailsByAccount(ctx, accountID, limit, offset)
}

func (s *Service) SyncAccount(ctx context.Context, accountID int64) (int, error) {
	account, err := s.repo.GetAccount(ctx, accountID)
	if err != nil {
		return 0, err
	}
	client, err := s.clientForProtocol(account.Protocol)
	if err != nil {
		return 0, err
	}
	remoteEmails, cursor, err := client.Fetch(ctx, account, account.LastSyncedCursor)
	if err != nil {
		return 0, fmt.Errorf("fetch remote emails: %w", err)
	}

	stored := 0
	for _, remote := range remoteEmails {
		path, sum, size, err := s.store.Store(ctx, account.ID, remote.RemoteID, remote.RawMIME)
		if err != nil {
			return stored, fmt.Errorf("store raw email %s: %w", remote.RemoteID, err)
		}
		_, err = s.repo.SaveEmailMetadata(ctx, domain.EmailMetadata{
			AccountID:      account.ID,
			RemoteID:       remote.RemoteID,
			Subject:        remote.Subject,
			FromAddress:    remote.FromAddress,
			ToAddress:      remote.ToAddress,
			ReceivedAt:     normalizeZeroTime(remote.ReceivedAt, s.now()),
			RawPath:        path,
			RawSHA256:      sum,
			RawSizeBytes:   size,
			Preview:        remote.Preview,
			StoredAt:       s.now().UTC(),
			MessageID:      remote.MessageID,
			ThreadID:       remote.ThreadID,
			InReplyTo:      remote.InReplyTo,
			References:     remote.References,
			HasAttachments: remote.HasAttachments,
		})
		if err != nil {
			return stored, fmt.Errorf("save email metadata %s: %w", remote.RemoteID, err)
		}
		stored++
	}
	if cursor != "" {
		if err := s.repo.UpdateAccountSyncCursor(ctx, account.ID, cursor); err != nil {
			return stored, fmt.Errorf("update sync cursor: %w", err)
		}
	}
	return stored, nil
}

func (s *Service) clientForProtocol(protocol domain.Protocol) (domain.EmailClient, error) {
	switch protocol {
	case domain.ProtocolIMAP:
		return s.client.IMAP, nil
	case domain.ProtocolPOP3:
		return s.client.POP3, nil
	default:
		return nil, ErrInvalidProtocol
	}
}

func validateAccount(account domain.Account) error {
	if account.Protocol != domain.ProtocolIMAP && account.Protocol != domain.ProtocolPOP3 {
		return ErrInvalidProtocol
	}
	if strings.TrimSpace(account.Name) == "" {
		return errors.New("account name is required")
	}
	if strings.TrimSpace(account.Host) == "" {
		return errors.New("host is required")
	}
	if account.Port <= 0 || account.Port > 65535 {
		return errors.New("port must be between 1 and 65535")
	}
	if strings.TrimSpace(account.Username) == "" {
		return errors.New("username is required")
	}
	if account.Password == "" {
		return errors.New("password is required")
	}
	return nil
}

func normalizeZeroTime(value, fallback time.Time) time.Time {
	if value.IsZero() {
		return fallback.UTC()
	}
	return value.UTC()
}
