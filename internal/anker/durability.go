package anker

import (
	"os"
	"path/filepath"
)

func syncDir(path string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	return f.Sync()
}
func syncTree(root string) error {
	var dirs []string
	e := filepath.WalkDir(root, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			dirs = append(dirs, path)
			return nil
		}
		f, e := os.Open(path)
		if e != nil {
			return e
		}
		e = f.Sync()
		ce := f.Close()
		if e != nil {
			return e
		}
		return ce
	})
	if e != nil {
		return e
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		if e = syncDir(dirs[i]); e != nil {
			return e
		}
	}
	return nil
}
