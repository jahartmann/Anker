package anker

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type credential struct {
	Hash string `json:"hash"`
}
type SessionInfo struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
	Expires   time.Time `json:"expires"`
}
type Auth struct {
	store    *Store
	mu       sync.Mutex
	attempts map[string][]time.Time
}

func NewAuth(s *Store) *Auth { return &Auth{store: s, attempts: map[string][]time.Time{}} }
func (a *Auth) SessionDuration() (time.Duration, error) {
	var v Settings
	err := a.store.Get("settings", "main", &v)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	days := v.SessionDays
	if days == 0 {
		days = 30
	}
	if days < 1 || days > 365 {
		return 0, errors.New("ungültige Sitzungsdauer")
	}
	return time.Duration(days) * 24 * time.Hour, nil
}
func txPut(tx *sql.Tx, bucket, id string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO records(bucket,id,value) VALUES(?,?,?) ON CONFLICT(bucket,id) DO UPDATE SET value=excluded.value`, bucket, id, b)
	return err
}
func revokeTx(tx *sql.Tx, id string) error {
	_, err := tx.Exec(`DELETE FROM records WHERE bucket='sessions' AND json_extract(value,'$.user_id')=?`, id)
	return err
}
func (a *Auth) transaction(fn func(*sql.Tx) error) error {
	tx, err := a.store.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit()
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
	if !validID(name) || utf8.RuneCountInString(password) < 12 || len(password) > 1024 {
		return errors.New("Benutzername ungültig oder Passwort muss 12 bis 1024 Zeichen enthalten")
	}
	if !validRole(role) {
		return errors.New("unbekannte Rolle")
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	var existing User
	err = a.store.Get("users", name, &existing)
	if err == nil {
		return errors.New("Benutzer existiert bereits")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if role == "admin" {
		secrets = true
	}
	return a.transaction(func(tx *sql.Tx) error {
		if err := txPut(tx, "credentials", name, credential{Hash: hash}); err != nil {
			return err
		}
		return txPut(tx, "users", name, User{ID: name, Name: name, Role: role, Secrets: secrets})
	})
}
func validRole(role string) bool { return role == "admin" || role == "restore" || role == "reader" }
func (a *Auth) SetPassword(id, password string) error {
	if utf8.RuneCountInString(password) < 12 || len(password) > 1024 {
		return errors.New("Passwort muss 12 bis 1024 Zeichen enthalten")
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	var u User
	if err = a.store.Get("users", id, &u); err != nil {
		return err
	}
	return a.transaction(func(tx *sql.Tx) error {
		if err := txPut(tx, "credentials", id, credential{Hash: hash}); err != nil {
			return err
		}
		return revokeTx(tx, id)
	})
}
func (a *Auth) Login(name, password string) (string, User, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	var u User
	var c credential
	err := a.store.Get("users", name, &u)
	cerr := a.store.Get("credentials", name, &c)
	// Run a password derivation even for an unknown account to avoid a cheap user-name oracle.
	hash := c.Hash
	if cerr != nil {
		hash = "pbkdf2$600000$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	}
	valid := len(password) <= 1024 && checkPassword(hash, password)
	if err != nil || cerr != nil || u.Disabled || !valid {
		return "", User{}, errors.New("Anmeldung fehlgeschlagen")
	}
	duration, err := a.SessionDuration()
	if err != nil {
		return "", User{}, err
	}
	bytes := make([]byte, 32)
	if _, err = rand.Read(bytes); err != nil {
		return "", User{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(bytes)
	at := time.Now().UTC()
	info := SessionInfo{ID: ID(), UserID: u.ID, CreatedAt: at, Expires: at.Add(duration)}
	err = a.transaction(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DELETE FROM records WHERE bucket='sessions' AND unixepoch(json_extract(value,'$.expires'))<=?`, at.Unix()); err != nil {
			return err
		}
		// Keep at most ten devices per account; remove the oldest session before adding another.
		if _, err := tx.Exec(`DELETE FROM records WHERE bucket='sessions' AND id IN (SELECT id FROM records WHERE bucket='sessions' AND json_extract(value,'$.user_id')=? ORDER BY json_extract(value,'$.created_at') DESC LIMIT -1 OFFSET 9)`, u.ID); err != nil {
			return err
		}
		return txPut(tx, "sessions", Hash([]byte(token)), info)
	})
	if err != nil {
		return "", User{}, err
	}
	return token, u, nil
}
func (a *Auth) Session(token string, at time.Time) (User, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(token) != 43 {
		return User{}, errors.New("Anmeldung erforderlich")
	}
	var v SessionInfo
	var u User
	key := Hash([]byte(token))
	if err := a.store.Get("sessions", key, &v); err != nil {
		return u, errors.New("Anmeldung erforderlich")
	}
	if !at.Before(v.Expires) {
		a.store.Delete("sessions", key)
		return u, errors.New("Anmeldung erforderlich")
	}
	if err := a.store.Get("users", v.UserID, &u); err != nil || u.Disabled {
		return User{}, errors.New("Anmeldung erforderlich")
	}
	return u, nil
}
func (a *Auth) Logout(token string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.store.Delete("sessions", Hash([]byte(token)))
}
func (a *Auth) RevokeSessions(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.transaction(func(tx *sql.Tx) error { return revokeTx(tx, id) })
}
func (a *Auth) Sessions(id string) ([]SessionInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	all, err := records[SessionInfo](a.store, "sessions")
	if err != nil {
		return nil, err
	}
	out := []SessionInfo{}
	for _, v := range all {
		if v.UserID == id && time.Now().Before(v.Expires) {
			out = append(out, v)
		}
	}
	return out, nil
}
func (a *Auth) UpdateUser(actor, id, role string, secrets, disabled bool) error {
	if !validRole(role) {
		return errors.New("unbekannte Rolle")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	var u User
	if err := a.store.Get("users", id, &u); err != nil {
		return err
	}
	if actor == id && (disabled || role != "admin") {
		return errors.New("Eigenen Administratorzugang nicht sperren oder herabstufen")
	}
	if err := a.protectLastAdmin(u, disabled || role != "admin"); err != nil {
		return err
	}
	if role == "admin" {
		secrets = true
	}
	if u.Role == role && u.Secrets == secrets && u.Disabled == disabled {
		return nil
	}
	u.Role, u.Secrets, u.Disabled = role, secrets, disabled
	return a.transaction(func(tx *sql.Tx) error {
		if err := txPut(tx, "users", id, u); err != nil {
			return err
		}
		return revokeTx(tx, id)
	})
}
func (a *Auth) protectLastAdmin(u User, removing bool) error {
	if !removing || u.Role != "admin" || u.Disabled {
		return nil
	}
	users, err := records[User](a.store, "users")
	if err != nil {
		return err
	}
	for _, other := range users {
		if other.ID != u.ID && other.Role == "admin" && !other.Disabled {
			return nil
		}
	}
	return errors.New("Mindestens ein aktiver Administrator muss erhalten bleiben")
}
func (a *Auth) DeleteUser(actor, id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if actor == id {
		return errors.New("Eigenen Benutzer nicht löschen")
	}
	var u User
	if err := a.store.Get("users", id, &u); err != nil {
		return err
	}
	if err := a.protectLastAdmin(u, true); err != nil {
		return err
	}
	return a.transaction(func(tx *sql.Tx) error {
		if err := revokeTx(tx, id); err != nil {
			return err
		}
		_, err := tx.Exec(`DELETE FROM records WHERE id=? AND bucket IN ('users','credentials')`, id)
		return err
	})
}
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
	if len(a.attempts) >= 4096 && len(a.attempts[key]) == 0 {
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
