//go:build !linux

package updater

import "errors"

func verifyStorageDevice(string, string, int64) error {
	return errors.New("Dateisystemerweiterung benötigt Linux")
}
