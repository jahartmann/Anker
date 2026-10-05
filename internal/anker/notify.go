package anker

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/smtp"
	"strings"
	"time"
)

func decodeJSON(b []byte, v any) error { return json.Unmarshal(b, v) }
func (s *Service) notifyJob(j Job) {
	if j.Kind != "backup" {
		return
	}
	settings, err := s.Settings()
	if err != nil {
		return
	}
	jobs, _ := s.Jobs()
	var previous *Job
	for _, v := range jobs {
		if v.ID != j.ID && v.HostID == j.HostID && v.Kind == "backup" && v.FinishedAt != "" && v.CreatedAt < j.CreatedAt && (previous == nil || v.CreatedAt > previous.CreatedAt) {
			x := v
			previous = &x
		}
	}
	if j.State == "successful" && (previous == nil || previous.State == "successful") {
		return
	}
	h, _ := s.Host(j.HostID)
	message := fmt.Sprintf("Anker: %s — %s (%s)", h.Name, j.State, j.ID)
	s.SendNotification(settings, message)
}
func (s *Service) SendNotification(settings Settings, message string) error {
	if settings.Webhook != "" {
		b, _ := json.Marshal(map[string]string{"text": message})
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Post(settings.Webhook, "application/json", bytes.NewReader(b))
		if err != nil {
			return fmt.Errorf("Webhook konnte nicht erreicht werden")
		}
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("Webhook HTTP %d", resp.StatusCode)
		}
	}
	if settings.SMTPServer != "" && settings.MailTo != "" {
		host, port, err := net.SplitHostPort(settings.SMTPServer)
		if err != nil {
			return err
		}
		for _, v := range []string{settings.MailFrom, settings.MailTo} {
			if strings.ContainsAny(v, "\r\n") {
				return fmt.Errorf("ungültige Mailadresse")
			}
		}
		config := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
		var conn net.Conn
		if port == "465" {
			conn, err = tls.DialWithDialer(&net.Dialer{Timeout: 10 * time.Second}, "tcp", settings.SMTPServer, config)
		} else {
			conn, err = net.DialTimeout("tcp", settings.SMTPServer, 10*time.Second)
		}
		if err != nil {
			return fmt.Errorf("SMTP-Verbindung fehlgeschlagen")
		}
		conn.SetDeadline(time.Now().Add(15 * time.Second))
		client, err := smtp.NewClient(conn, host)
		if err != nil {
			conn.Close()
			return err
		}
		defer client.Close()
		if port != "465" {
			if err = client.StartTLS(config); err != nil {
				return fmt.Errorf("SMTP benötigt TLS")
			}
		}
		if settings.SMTPUser != "" {
			if err = client.Auth(smtp.PlainAuth("", settings.SMTPUser, settings.SMTPPassword, host)); err != nil {
				return fmt.Errorf("SMTP-Anmeldung fehlgeschlagen")
			}
		}
		if err = client.Mail(settings.MailFrom); err != nil {
			return err
		}
		if err = client.Rcpt(settings.MailTo); err != nil {
			return err
		}
		w, err := client.Data()
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(w, "From: %s\r\nTo: %s\r\nSubject: Anker Sicherungsstatus\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s\r\n", settings.MailFrom, settings.MailTo, message)
		if err == nil {
			err = w.Close()
		} else {
			w.Close()
		}
		return err
	}
	return nil
}
