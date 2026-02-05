package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

var invalidPathChars = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

type DiskRawEmailStore struct {
	rootDir string
}

func NewDiskRawEmailStore(rootDir string) (*DiskRawEmailStore, error) {
	if rootDir == "" {
		return nil, fmt.Errorf("raw store root directory is required")
	}
	if err := os.MkdirAll(rootDir, 0o750); err != nil {
		return nil, fmt.Errorf("ensure raw store root directory: %w", err)
	}
	return &DiskRawEmailStore{rootDir: rootDir}, nil
}

func (s *DiskRawEmailStore) Store(_ context.Context, accountID int64, remoteID string, raw []byte) (string, string, int64, error) {
	sum := sha256.Sum256(raw)
	hexSum := hex.EncodeToString(sum[:])
	sanitizedRemoteID := invalidPathChars.ReplaceAllString(remoteID, "_")
	if sanitizedRemoteID == "" {
		sanitizedRemoteID = hexSum[:16]
	}
	accountDir := filepath.Join(s.rootDir, fmt.Sprintf("account_%d", accountID))
	if err := os.MkdirAll(accountDir, 0o750); err != nil {
		return "", "", 0, fmt.Errorf("create account directory: %w", err)
	}
	filename := fmt.Sprintf("%s_%s.eml", sanitizedRemoteID, hexSum[:12])
	fullPath := filepath.Join(accountDir, filename)
	if err := os.WriteFile(fullPath, raw, 0o600); err != nil {
		return "", "", 0, fmt.Errorf("write raw email: %w", err)
	}
	return fullPath, hexSum, int64(len(raw)), nil
}
