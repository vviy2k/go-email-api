package mail

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"strings"
	"time"

	"go-email-api/internal/domain"
)

type POP3Client struct{}

func NewPOP3Client() *POP3Client { return &POP3Client{} }

func (c *POP3Client) Fetch(ctx context.Context, account domain.Account, cursor string) ([]domain.RemoteEmail, string, error) {
	addr := fmt.Sprintf("%s:%d", account.Host, account.Port)
	dialer := net.Dialer{Timeout: 10 * time.Second}
	var conn net.Conn
	var err error
	if account.UseTLS {
		conn, err = tls.DialWithDialer(&dialer, "tcp", addr, &tls.Config{ServerName: account.Host, MinVersion: tls.VersionTLS12})
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return nil, cursor, err
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	tp := textproto.NewConn(conn)
	defer tp.Close()
	if _, err := tp.ReadLine(); err != nil {
		return nil, cursor, fmt.Errorf("pop3 greeting: %w", err)
	}
	if err := expectOK(tp.PrintfLine("USER %s", account.Username), tp); err != nil {
		return nil, cursor, fmt.Errorf("pop3 user: %w", err)
	}
	if err := expectOK(tp.PrintfLine("PASS %s", account.Password), tp); err != nil {
		return nil, cursor, fmt.Errorf("pop3 pass: %w", err)
	}
	if err := expectOK(tp.PrintfLine("UIDL"), tp); err != nil {
		return nil, cursor, fmt.Errorf("pop3 uidl: %w", err)
	}
	uidLines, err := readMultiline(tp.R)
	if err != nil {
		return nil, cursor, err
	}
	lastSeen := strings.TrimSpace(cursor)
	newCursor := lastSeen
	remote := make([]domain.RemoteEmail, 0)
	for _, line := range uidLines {
		parts := strings.Fields(line)
		if len(parts) != 2 {
			continue
		}
		msgNum, uid := parts[0], parts[1]
		if uid <= lastSeen {
			continue
		}
		if err := expectOK(tp.PrintfLine("RETR %s", msgNum), tp); err != nil {
			return nil, newCursor, fmt.Errorf("pop3 retr %s: %w", msgNum, err)
		}
		rawLines, err := readMultiline(tp.R)
		if err != nil {
			return nil, newCursor, err
		}
		raw := []byte(strings.Join(rawLines, "\r\n") + "\r\n")
		subject, fromAddr, toAddr, messageID, inReplyTo, references, receivedAt := parseEnvelope(raw)
		remote = append(remote, domain.RemoteEmail{
			RemoteID:       uid,
			MessageID:      messageID,
			InReplyTo:      inReplyTo,
			References:     references,
			Subject:        subject,
			FromAddress:    fromAddr,
			ToAddress:      toAddr,
			ReceivedAt:     normalizeMessageTime(receivedAt),
			Preview:        buildPreview(raw),
			RawMIME:        raw,
			HasAttachments: hasAttachments(raw),
			Cursor:         uid,
		})
		if uid > newCursor {
			newCursor = uid
		}
	}
	_ = expectOK(tp.PrintfLine("QUIT"), tp)
	return remote, newCursor, nil
}

func expectOK(cmdErr error, tp *textproto.Conn) error {
	if cmdErr != nil {
		return cmdErr
	}
	line, err := tp.ReadLine()
	if err != nil {
		return err
	}
	if !strings.HasPrefix(line, "+OK") {
		return fmt.Errorf("unexpected response: %s", line)
	}
	return nil
}

func readMultiline(r *bufio.Reader) ([]string, error) {
	lines := make([]string, 0)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "." {
			break
		}
		if strings.HasPrefix(line, "..") {
			line = strings.TrimPrefix(line, ".")
		}
		lines = append(lines, line)
	}
	return lines, nil
}
