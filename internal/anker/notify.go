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
	if settings.Webhook == "" && (settings.SMTPServer == "" || settings.MailTo == "") {
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
	s.sendOperationalNotification(settings, message)
}

type staleNotice struct {
	Alerted   bool   `json:"alerted"`
	AttemptAt string `json:"attempt_at"`
}

// One overdue/recovery pair per incident, persisted across daemon restarts.
func (s *Service) CheckStaleBackups(at time.Time) error {
	s.notifyMu.Lock()
	defer s.notifyMu.Unlock()
	settings, err := s.Settings()
	if err != nil {
		return err
	}
	if settings.Webhook == "" && (settings.SMTPServer == "" || settings.MailTo == "") {
		return nil
	}
	hosts, err := s.Hosts()
	if err != nil {
		return err
	}
	for _, h := range hosts {
		if !h.Enabled {
			continue
		}
		backups, err := s.ListBackups(h.ID)
		if err != nil {
			return err
		}
		stale := true
		for _, b := range backups {
			if b.Status != "successful" {
				continue
			}
			created, err := time.Parse(time.RFC3339Nano, b.CreatedAt)
			if err == nil {
				stale = at.Sub(created) > time.Duration(settings.StaleHours)*time.Hour
			}
			break
		}
		var notice staleNotice
		s.Store.Get("stale_notifications", h.ID, &notice)
		if notice.Alerted == stale {
			continue
		}
		if previous, err := time.Parse(time.RFC3339Nano, notice.AttemptAt); err == nil && at.Sub(previous) < time.Hour {
			continue
		}
		message := "Anker: " + h.Name + " — Sicherung überfällig oder noch kein vollständiger Stand vorhanden"
		if !stale {
			message = "Anker: " + h.Name + " — wieder aktuell gesichert"
		}
		err = s.sendOperationalNotification(settings, message)
		if err == nil {
			notice.Alerted = stale
			notice.AttemptAt = ""
		} else {
			notice.AttemptAt = at.Format(time.RFC3339Nano)
		}
		if putErr := s.Store.Put("stale_notifications", h.ID, notice); putErr != nil {
			return putErr
		}
		if err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) sendOperationalNotification(settings Settings, message string) error {
	err := s.SendNotification(settings, message)
	state := map[string]string{"at": now(), "error": ""}
	if err != nil {
		state["error"] = err.Error()
	}
	if putErr := s.Store.Put("health", "notifications", state); err == nil {
		err = putErr
	}
	return err
}
func (s *Service) SendNotification(settings Settings, message string) error {
	if settings.Webhook == "" && (settings.SMTPServer == "" || settings.MailTo == "") {
		return fmt.Errorf("Kein Benachrichtigungsziel eingerichtet")
	}
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
