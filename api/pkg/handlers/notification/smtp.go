package notification

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"
)

type SMTPClient interface {
	SendMail(string, string, string, string, string, []string, []byte) error
}
type SMTPTransport struct {
	Host, Port, Username, Password, From string
	To                                   []string
	Client                               SMTPClient
}

func (s SMTPTransport) Notify(ctx context.Context, m Message) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if s.Client == nil {
		return fmt.Errorf("SMTP client is required")
	}
	msg := []byte("From: " + s.From + "\r\nTo: " + s.To[0] + "\r\nSubject: " + m.Subject + "\r\n\r\n" + m.Body + "\r\n")
	if err := s.Client.SendMail(s.Host, s.Port, s.Username, s.Password, s.From, s.To, msg); err != nil {
		return fmt.Errorf("send SMTP notification: %w", err)
	}
	return nil
}

type Client struct{ TLSConfig *tls.Config }

func (c Client) SendMail(host, port, username, password, from string, to []string, msg []byte) error {
	conn, err := tls.Dial("tcp", host+":"+port, c.TLSConfig)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	read := func() error {
		b := make([]byte, 512)
		n, e := conn.Read(b)
		if e != nil {
			return e
		}
		if len(b) > 0 && b[0] >= '4' {
			return fmt.Errorf("SMTP: %s", strings.TrimSpace(string(b[:n])))
		}
		return nil
	}
	write := func(v string) error { _, e := conn.Write([]byte(v + "\r\n")); return e }
	if err = read(); err != nil {
		return err
	}
	if err = write("EHLO localhost"); err != nil {
		return err
	}
	if err = read(); err != nil {
		return err
	}
	if username != "" {
		if err = write("AUTH PLAIN"); err != nil {
			return err
		}
		if err = read(); err != nil {
			return err
		}
	}
	if err = write("MAIL FROM:<" + from + ">"); err != nil {
		return err
	}
	if err = read(); err != nil {
		return err
	}
	for _, recipient := range to {
		if err = write("RCPT TO:<" + recipient + ">"); err != nil {
			return err
		}
		if err = read(); err != nil {
			return err
		}
	}
	if err = write("DATA"); err != nil {
		return err
	}
	if err = read(); err != nil {
		return err
	}
	if _, err = conn.Write(append(msg, []byte("\r\n.\r\n")...)); err != nil {
		return err
	}
	return read()
}

type TLSConfig struct {
	Host               string
	InsecureSkipVerify bool
}

func (c TLSConfig) Config() *tls.Config {
	return &tls.Config{ServerName: c.Host, InsecureSkipVerify: c.InsecureSkipVerify}
}
