package mail

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/textproto"
	"sort"
	"strconv"
	"strings"
	"time"

	"go-email-api/internal/domain"
)

type IMAPClient struct{}

func NewIMAPClient() *IMAPClient { return &IMAPClient{} }

func (c *IMAPClient) Fetch(ctx context.Context, account domain.Account, cursor string) ([]domain.RemoteEmail, string, error) {
	addr := fmt.Sprintf("%s:%d", account.Host, account.Port)
	dialer := net.Dialer{Timeout: 12 * time.Second}
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
	tp := textproto.NewConn(conn)
	defer tp.Close()
	if _, err := tp.ReadLine(); err != nil {
		return nil, cursor, fmt.Errorf("imap greeting: %w", err)
	}
	mailbox := account.Mailbox
	if mailbox == "" {
		mailbox = "INBOX"
	}
	if _, err := sendIMAP(tp, `LOGIN `+quoteIMAP(account.Username)+` `+quoteIMAP(account.Password)); err != nil {
		return nil, cursor, err
	}
	if _, err := sendIMAP(tp, `SELECT `+quoteIMAP(mailbox)); err != nil {
		return nil, cursor, err
	}

	startUID := parseUID(cursor) + 1
	criteria := "1:*"
	if startUID > 1 {
		criteria = fmt.Sprintf("%d:*", startUID)
	}
	lines, err := sendIMAP(tp, fmt.Sprintf("UID SEARCH UID %s", criteria))
	if err != nil {
		return nil, cursor, err
	}
	uids := parseSearchUIDs(lines)
	if len(uids) == 0 {
		return nil, cursor, nil
	}
	sort.Slice(uids, func(i, j int) bool { return uids[i] < uids[j] })

	remote := make([]domain.RemoteEmail, 0, len(uids))
	newCursor := cursor
	for _, uid := range uids {
		select {
		case <-ctx.Done():
			return nil, cursor, ctx.Err()
		default:
		}
		raw, err := fetchRFC822(tp, uid)
		if err != nil {
			return nil, newCursor, err
		}
		subject, fromAddr, toAddr, messageID, inReplyTo, references, receivedAt := parseEnvelope(raw)
		uidStr := strconv.FormatUint(uint64(uid), 10)
		remote = append(remote, domain.RemoteEmail{
			RemoteID:       uidStr,
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
			Cursor:         uidStr,
		})
		newCursor = uidStr
	}
	_, _ = sendIMAP(tp, "LOGOUT")
	return remote, newCursor, nil
}

func sendIMAP(tp *textproto.Conn, command string) ([]string, error) {
	tag := "A001"
	if err := tp.PrintfLine("%s %s", tag, command); err != nil {
		return nil, err
	}
	lines := make([]string, 0)
	for {
		line, err := tp.ReadLine()
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(line, tag+" ") {
			if !strings.Contains(line, " OK") {
				return nil, fmt.Errorf("imap command failed: %s", line)
			}
			return lines, nil
		}
		lines = append(lines, line)
	}
}

func fetchRFC822(tp *textproto.Conn, uid uint32) ([]byte, error) {
	tag := "A001"
	if err := tp.PrintfLine("%s UID FETCH %d (RFC822)", tag, uid); err != nil {
		return nil, err
	}
	reader := tp.R
	var raw []byte
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(line, "*") && strings.Contains(line, "RFC822 {") {
			size, err := parseLiteralSize(line)
			if err != nil {
				return nil, err
			}
			raw = make([]byte, size)
			if _, err := ioReadFull(reader, raw); err != nil {
				return nil, err
			}
			_, _ = reader.ReadString('\n')
			continue
		}
		if strings.HasPrefix(line, tag+" ") {
			if !strings.Contains(line, " OK") {
				return nil, fmt.Errorf("imap fetch failed: %s", line)
			}
			break
		}
	}
	return raw, nil
}

func parseLiteralSize(line string) (int, error) {
	start := strings.LastIndex(line, "{")
	end := strings.LastIndex(line, "}")
	if start == -1 || end == -1 || end <= start+1 {
		return 0, fmt.Errorf("invalid literal line: %s", line)
	}
	return strconv.Atoi(strings.TrimSpace(line[start+1 : end]))
}

func parseSearchUIDs(lines []string) []uint32 {
	uids := make([]uint32, 0)
	for _, line := range lines {
		if !strings.HasPrefix(line, "* SEARCH") {
			continue
		}
		parts := strings.Fields(strings.TrimPrefix(line, "* SEARCH"))
		for _, p := range parts {
			v, err := strconv.ParseUint(strings.TrimSpace(p), 10, 32)
			if err == nil {
				uids = append(uids, uint32(v))
			}
		}
	}
	return uids
}

func parseUID(cursor string) uint32 {
	v, _ := strconv.ParseUint(strings.TrimSpace(cursor), 10, 32)
	return uint32(v)
}

func quoteIMAP(v string) string {
	return `"` + strings.ReplaceAll(v, `"`, `\\"`) + `"`
}

func ioReadFull(r *bufio.Reader, b []byte) (int, error) {
	total := 0
	for total < len(b) {
		n, err := r.Read(b[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
