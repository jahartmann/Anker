package anker

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

type credential struct {
	Hash string `json:"hash"`
}
type session struct {
	UserID  string
	Expires time.Time
}
type Auth struct {
	store    *Store
	mu       sync.Mutex
	sessions map[string]session
	attempts map[string][]time.Time
}

func NewAuth(s *Store) *Auth {
	return &Auth{store: s, sessions: map[string]session{}, attempts: map[string][]time.Time{}}
}
func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, 600000, 32)
	if err != nil {
		return "", err
	}
	return "pbkdf2$600000$" + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(key), nil
}
func checkPassword(hash, password string) bool {
	p := strings.Split(hash, "$")
	if len(p) != 4 || p[0] != "pbkdf2" {
		return false
	}
	rounds, err := strconv.Atoi(p[1])
	if err != nil || rounds < 100000 || rounds > 1000000 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(p[2])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(p[3])
	if err != nil || len(want) != 32 {
		return false
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, rounds, 32)
	return err == nil && subtle.ConstantTimeCompare(want, key) == 1
}
func (a *Auth) CreateUser(name, password, role string, secrets bool) error {
	if !validID(name) || len(password) < 12 || len(password) > 1024 {
		return errors.New("Benutzername ungültig oder Passwort kürzer als 12 Zeichen")
	}
	if role != "admin" && role != "restore" && role != "reader" {
		return errors.New("unbekannte Rolle")
	}
	var existing User
	if a.store.Get("users", name, &existing) == nil {
		return errors.New("Benutzer existiert bereits")
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	if role == "admin" {
		secrets = true
	}
	if err = a.store.Put("credentials", name, credential{Hash: hash}); err != nil {
		return err
	}
	return a.store.Put("users", name, User{ID: name, Name: name, Role: role, Secrets: secrets})
}
func (a *Auth) SetPassword(id, password string) error {
	if len(password) < 12 || len(password) > 1024 {
		return errors.New("Passwort muss mindestens 12 Zeichen enthalten")
	}
	var u User
	if err := a.store.Get("users", id, &u); err != nil {
		return err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	if err = a.store.Put("credentials", id, credential{Hash: hash}); err != nil {
		return err
	}
	a.mu.Lock()
	for key, v := range a.sessions {
		if v.UserID == id {
			delete(a.sessions, key)
		}
	}
	a.mu.Unlock()
	return nil
}
func (a *Auth) Login(name, password string) (string, User, error) {
	var u User
	var c credential
	err := a.store.Get("users", name, &u)
	cerr := a.store.Get("credentials", name, &c)
	if err != nil || cerr != nil || u.Disabled || !checkPassword(c.Hash, password) {
		return "", User{}, errors.New("Anmeldung fehlgeschlagen")
	}
	token := ID() + ID()
	a.mu.Lock()
	defer a.mu.Unlock()
	for token, v := range a.sessions {
		if !time.Now().Before(v.Expires) {
			delete(a.sessions, token)
		}
	}
	a.sessions[token] = session{UserID: u.ID, Expires: time.Now().Add(8 * time.Hour)}
	return token, u, nil
}
func (a *Auth) Session(token string, at time.Time) (User, error) {
	a.mu.Lock()
	v, ok := a.sessions[token]
	if ok && !at.Before(v.Expires) {
		delete(a.sessions, token)
		ok = false
	}
	a.mu.Unlock()
	var u User
	if !ok {
		return u, errors.New("Anmeldung erforderlich")
	}
	err := a.store.Get("users", v.UserID, &u)
	if err != nil || u.Disabled {
		return User{}, errors.New("Anmeldung erforderlich")
	}
	return u, nil
}
func (a *Auth) Logout(token string) { a.mu.Lock(); delete(a.sessions, token); a.mu.Unlock() }
func (a *Auth) PermitLogin(key string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	cut := time.Now().Add(-15 * time.Minute)
	for key, times := range a.attempts {
		if len(times) == 0 || !times[len(times)-1].After(cut) {
			delete(a.attempts, key)
		}
	}
	v := []time.Time{}
	for _, at := range a.attempts[key] {
		if at.After(cut) {
			v = append(v, at)
		}
	}
	if len(v) >= 8 {
		return false
	}
	v = append(v, time.Now())
	a.attempts[key] = v
	return true
}
func (s *Service) LogAudit(user, action, object string) error {
	v := Audit{ID: ID(), At: now(), User: user, Action: action, Object: object}
	return s.Store.Put("audit", v.ID, v)
}
func userAllows(u User, action string) bool {
	switch action {
	case "admin":
		return u.Role == "admin"
	case "restore":
		return u.Role == "admin" || u.Role == "restore"
	case "secrets":
		return u.Secrets
	default:
		return u.Role != ""
	}
}
func deny(action string) error { return fmt.Errorf("Berechtigung erforderlich: %s", action) }
