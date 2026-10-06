package anker

import (
	"anker/internal/updater"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

func (s *Service) certificates(w http.ResponseWriter, r *http.Request, u User) error {
	if err := require(u, "admin"); err != nil {
		return err
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/tls")
	valid := (path == "" && (r.Method == "GET" || r.Method == "POST")) || (path == "/renew" && r.Method == "POST") || (path == "/certificate" && r.Method == "GET")
	if !valid {
		return fail(405, "Methode nicht unterstützt")
	}
	if s.Demo {
		if r.Method == "GET" && path == "" {
			return jsonOut(w, updater.TLSStatus{RenewBeforeDays: 30, Message: "Zertifikatsverwaltung ist in der lokalen Demo ausgeschaltet."})
		}
		return fail(409, "Zertifikatsverwaltung ist in der lokalen Demo ausgeschaltet")
	}
	var body io.Reader
	if r.Method == "POST" {
		var target any
		if path == "/renew" {
			target = &struct{}{}
		} else {
			target = &struct {
				Automatic       bool `json:"automatic"`
				RenewBeforeDays int  `json:"renew_before_days"`
			}{}
		}
		if err := input(r, target); err != nil {
			return err
		}
		data, _ := json.Marshal(target)
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, "http://updater.local/tls"+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("X-Anker-Request", "1")
	response, err := updater.UnixClient(updater.Socket, 10*time.Second).Do(req)
	if err != nil {
		return fail(503, "Zertifikatsverwaltung nicht erreichbar; anker-updater.service und Servereinrichtung prüfen")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil {
		return err
	}
	if len(data) > 1<<20 {
		return fail(502, "Ungültige Antwort der Zertifikatsverwaltung")
	}
	if response.StatusCode >= 400 {
		var failure struct {
			Error string `json:"error"`
		}
		if err = json.Unmarshal(data, &failure); err != nil {
			return err
		}
		return fail(response.StatusCode, failure.Error)
	}
	if path == "/certificate" {
		if r.URL.Query().Get("check") == "1" {
			return jsonOut(w, map[string]bool{"ok": true})
		}
		w.Header().Set("Content-Type", "application/x-pem-file")
		w.Header().Set("Content-Disposition", `attachment; filename="anker-server.crt"`)
		_, err = w.Write(data)
		return err
	}
	var status updater.TLSStatus
	if err = json.Unmarshal(data, &status); err != nil {
		return err
	}
	if r.Method == "POST" {
		action := "tls.policy"
		if path == "/renew" {
			action = "tls.renew"
		}
		s.LogAudit(u.ID, action, status.Fingerprint)
	}
	return jsonOut(w, status)
}
