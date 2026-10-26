package anker

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type demoMutationFile struct {
	Path      string
	BeforeSHA string
	AfterSHA  string
	Mode      uint32
	Phase     string
}
type demoMutationJournal struct {
	State string
	Files []demoMutationFile
}

func demoState(root, path string) (string, error) {
	p, err := safeJoin(root, path)
	if err != nil {
		return "", err
	}
	st, err := os.Lstat(p)
	if os.IsNotExist(err) {
		return "missing", nil
	}
	if err != nil {
		return "", err
	}
	if !st.Mode().IsRegular() {
		return "", errors.New("Demo-Ziel ist keine gewöhnliche Datei")
	}
	b, err := os.ReadFile(p)
	return Hash(b), err
}
func (c DemoCollector) demoJournal(h Host, p Plan) (string, demoMutationJournal, error) {
	root, err := c.root(h)
	if err != nil {
		return "", demoMutationJournal{}, err
	}
	folder := filepath.Join(root, "rollback", p.ID)
	raw, err := os.ReadFile(filepath.Join(folder, "journal.json"))
	var j demoMutationJournal
	if err == nil {
		err = json.Unmarshal(raw, &j)
	}
	return folder, j, err
}
func demoResult(p Plan, folder string, j demoMutationJournal) ApplyResult {
	r := ApplyResult{State: j.State, OperationID: p.ID, RollbackPath: folder, Applied: []string{}, Checks: []string{"Lokales Demojournal · kein realer Hostnachweis"}}
	for _, f := range j.Files {
		if f.Phase == "applied" || f.Phase == "rolled_back" || f.Phase == "conflict" {
			r.Applied = append(r.Applied, f.Path)
		}
	}
	return r
}
func (c DemoCollector) applyRemovalPlan(ctx context.Context, h Host, p Plan, planRoot string) (ApplyResult, error) {
	root, err := c.root(h)
	if err != nil {
		return ApplyResult{}, err
	}
	target := filepath.Join(root, "files")
	folder := filepath.Join(root, "rollback", p.ID)
	if older, readErr := os.ReadDir(filepath.Dir(folder)); readErr == nil {
		for _, entry := range older {
			if !entry.IsDir() {
				continue
			}
			raw, e := os.ReadFile(filepath.Join(filepath.Dir(folder), entry.Name(), "journal.json"))
			if e != nil {
				continue
			}
			var previous demoMutationJournal
			if json.Unmarshal(raw, &previous) != nil || previous.State == "writing" || previous.State == "interrupted" || previous.State == "rolling_back" || previous.State == "rollback_conflict" {
				return ApplyResult{}, errors.New("Ungeklärtes Demojournal zuerst abgleichen und zurücksetzen")
			}
		}
	}
	if err = os.Mkdir(folder, 0700); os.IsNotExist(err) {
		if err = os.MkdirAll(filepath.Dir(folder), 0700); err == nil {
			err = os.Mkdir(folder, 0700)
		}
	}
	if err != nil {
		return ApplyResult{}, errors.New("Demojournal existiert bereits oder kann nicht angelegt werden; keine Wiederholung")
	}
	j := demoMutationJournal{State: "writing"}
	save := func() error { return writeJSON(filepath.Join(folder, "journal.json"), j) }
	if err = save(); err != nil {
		return ApplyResult{}, err
	}
	for _, step := range p.Steps {
		if step.Action != "apply" && step.Action != "remove" {
			continue
		}
		sha, e := demoState(target, step.Path)
		if e != nil {
			return demoResult(p, folder, j), e
		}
		if sha != step.BeforeSHA {
			return demoResult(p, folder, j), errors.New("Demo-Ziel wurde verändert")
		}
		dest, _ := safeJoin(target, step.Path)
		mode := uint32(0644)
		if sha != "missing" {
			old, e := os.ReadFile(dest)
			if e != nil {
				return demoResult(p, folder, j), e
			}
			st, e := os.Stat(dest)
			if e != nil {
				return demoResult(p, folder, j), e
			}
			mode = uint32(st.Mode().Perm())
			back, _ := safeJoin(filepath.Join(folder, "before"), step.Path)
			if e = atomicWrite(back, old, 0600); e != nil {
				return demoResult(p, folder, j), e
			}
		}
		after := step.PreparedSHA
		if step.Action == "remove" {
			after = "missing"
		}
		j.Files = append(j.Files, demoMutationFile{Path: step.Path, BeforeSHA: sha, AfterSHA: after, Mode: mode, Phase: "prepared"})
	}
	if err = save(); err != nil {
		return demoResult(p, folder, j), err
	}
	if err = syncTree(folder); err != nil {
		return demoResult(p, folder, j), err
	}
	for i := range j.Files {
		f := &j.Files[i]
		if err = ctx.Err(); err != nil {
			break
		}
		var sha string
		sha, err = demoState(target, f.Path)
		if err != nil {
			break
		}
		if sha != f.BeforeSHA {
			err = errors.New("Demo-Ziel vor Änderung verändert")
			break
		}
		f.Phase = "writing"
		if err = save(); err != nil {
			break
		}
		dest, _ := safeJoin(target, f.Path)
		if f.AfterSHA == "missing" {
			err = os.Remove(dest)
			if err == nil {
				err = syncDir(filepath.Dir(dest))
			}
		} else {
			src, _ := safeJoin(filepath.Join(planRoot, "prepared-files"), f.Path)
			var data []byte
			data, err = os.ReadFile(src)
			if err == nil && Hash(data) != f.AfterSHA {
				err = errors.New("Vorbereitete Datei verändert")
			}
			if err == nil {
				err = atomicWrite(dest, data, 0600)
			}
		}
		if err != nil {
			break
		}
		f.Phase = "applied"
		if err = save(); err != nil {
			break
		}
	}
	if err != nil {
		j.State = "interrupted"
		_ = save()
		rolled, rollbackErr := c.Rollback(ctx, h, p, p.ID)
		return rolled, errors.Join(err, rollbackErr)
	}
	j.State = "applied"
	err = save()
	return demoResult(p, folder, j), err
}
func (c DemoCollector) RestoreStatus(_ context.Context, h Host, p Plan) (ApplyResult, error) {
	folder, j, err := c.demoJournal(h, p)
	if err != nil {
		return ApplyResult{}, err
	}
	if j.State == "writing" {
		j.State = "interrupted"
	}
	return demoResult(p, folder, j), nil
}
func (c DemoCollector) Rollback(_ context.Context, h Host, p Plan, confirmation string) (ApplyResult, error) {
	if confirmation != p.ID {
		return ApplyResult{}, errors.New("Plan-ID zur Rücksetzung bestätigen")
	}
	folder, j, err := c.demoJournal(h, p)
	if err != nil {
		return ApplyResult{}, err
	}
	root, _ := c.root(h)
	target := filepath.Join(root, "files")
	j.State = "rolling_back"
	if err = writeJSON(filepath.Join(folder, "journal.json"), j); err != nil {
		return ApplyResult{}, err
	}
	conflict := false
	for i := len(j.Files) - 1; i >= 0; i-- {
		f := &j.Files[i]
		if f.Phase == "prepared" {
			continue
		}
		current, e := demoState(target, f.Path)
		if e != nil || (current != f.AfterSHA && current != f.BeforeSHA) {
			conflict = true
			f.Phase = "conflict"
			continue
		}
		if current != f.BeforeSHA {
			dest, _ := safeJoin(target, f.Path)
			if f.BeforeSHA == "missing" {
				e = os.Remove(dest)
			} else {
				back, _ := safeJoin(filepath.Join(folder, "before"), f.Path)
				var data []byte
				data, e = os.ReadFile(back)
				if e == nil && Hash(data) != f.BeforeSHA {
					e = errors.New("Rücksetzdatei verändert")
				}
				if e == nil {
					e = atomicWrite(dest, data, os.FileMode(f.Mode))
				}
			}
			if e != nil {
				conflict = true
				f.Phase = "conflict"
				continue
			}
		}
		f.Phase = "rolled_back"
	}
	j.State = "rolled_back"
	if conflict {
		j.State = "rollback_conflict"
	}
	err = writeJSON(filepath.Join(folder, "journal.json"), j)
	return demoResult(p, folder, j), err
}
