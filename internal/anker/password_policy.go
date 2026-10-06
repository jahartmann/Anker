package anker

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"
)

const DefaultPasswordMinLength = 8

func PasswordMinimum(v Settings) (int, error) {
	minimum := v.PasswordMinLength
	if minimum == 0 {
		minimum = DefaultPasswordMinLength
	}
	if minimum < 8 || minimum > 128 {
		return 0, errors.New("Passwort-Mindestlänge muss zwischen 8 und 128 Zeichen liegen")
	}
	return minimum, nil
}

func ValidatePassword(password string, minimum int) error {
	minimum, err := PasswordMinimum(Settings{PasswordMinLength: minimum})
	if err != nil {
		return err
	}
	if utf8.RuneCountInString(password) < minimum || len(password) > 1024 {
		return fmt.Errorf("Passwort muss mindestens %d Zeichen und höchstens 1024 Bytes enthalten", minimum)
	}
	return nil
}

// Read through the credential transaction as well as before hashing so a
// concurrently increased policy cannot be bypassed at credential commit.
func readPasswordMinimum(db interface{ QueryRow(string, ...any) *sql.Row }) (int, error) {
	var data []byte
	err := db.QueryRow(`SELECT value FROM records WHERE bucket='settings' AND id='main'`).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultPasswordMinLength, nil
	}
	if err != nil {
		return 0, fmt.Errorf("Passwortvorgabe nicht lesbar: %w", err)
	}
	var v Settings
	if err = json.Unmarshal(data, &v); err != nil {
		return 0, fmt.Errorf("Passwortvorgabe nicht lesbar: %w", err)
	}
	return PasswordMinimum(v)
}
func (a *Auth) PasswordMinimum() (int, error) { return readPasswordMinimum(a.store.db) }
func (a *Auth) validatePassword(password string) error {
	minimum, err := a.PasswordMinimum()
	if err != nil {
		return err
	}
	return ValidatePassword(password, minimum)
}
func validatePasswordTx(tx *sql.Tx, password string) error {
	minimum, err := readPasswordMinimum(tx)
	if err != nil {
		return err
	}
	return ValidatePassword(password, minimum)
}
