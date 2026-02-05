package mail

import (
	"bytes"
	"io"
	stdmail "net/mail"
	"strings"
	"time"
)

func parseEnvelope(raw []byte) (subject, fromAddr, toAddr, messageID, inReplyTo, references string, receivedAt time.Time) {
	msg, err := stdmail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return
	}
	header := msg.Header
	subject = header.Get("Subject")
	messageID = header.Get("Message-ID")
	inReplyTo = header.Get("In-Reply-To")
	references = header.Get("References")
	if d, err := header.Date(); err == nil {
		receivedAt = d
	}
	if from, err := header.AddressList("From"); err == nil && len(from) > 0 {
		fromAddr = from[0].Address
	}
	if to, err := header.AddressList("To"); err == nil && len(to) > 0 {
		toAddr = to[0].Address
	}
	return
}

func buildPreview(raw []byte) string {
	msg, err := stdmail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return ""
	}
	body, _ := io.ReadAll(io.LimitReader(msg.Body, 2000))
	preview := strings.TrimSpace(string(body))
	preview = strings.ReplaceAll(preview, "\n", " ")
	preview = strings.ReplaceAll(preview, "\r", " ")
	if len(preview) > 250 {
		return preview[:250]
	}
	return preview
}

func hasAttachments(raw []byte) bool {
	msg, err := stdmail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return false
	}
	contentType := strings.ToLower(msg.Header.Get("Content-Type"))
	return strings.Contains(contentType, "multipart/mixed") || strings.Contains(contentType, "name=")
}

func normalizeMessageTime(receivedAt time.Time) time.Time {
	if receivedAt.IsZero() {
		return time.Now().UTC()
	}
	return receivedAt.UTC()
}
