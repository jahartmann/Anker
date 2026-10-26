package anker

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Fixed SSH transport fixture runs the repository's real helper functions against
// temporary files. Only probe commands/OS privilege checks are replaced: the real
// read, collect, apply, metadata checks, journal and rollback implementations run.
func localRecoveryHelper(t *testing.T) (*Service, PlanRequest, string) {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	helper, _ := filepath.Abs("../../host/anker_host.py")
	dir := t.TempDir()
	root := filepath.Join(dir, "host")
	path := "etc/sysctl.d/a.conf"
	if err = os.MkdirAll(filepath.Join(root, "etc/sysctl.d"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, path), []byte("vm.swappiness=10\n"), 0640); err != nil {
		t.Fatal(err)
	}
	driver := `import importlib.util,json,os,pathlib,sys,tarfile,tempfile,sqlite3,time
spec=importlib.util.spec_from_file_location('h',os.environ['ANKER_LOCAL_HELPER']);h=importlib.util.module_from_spec(spec);spec.loader.exec_module(h)
# macOS runtime lacks Python xattr APIs; fixtures contain no xattrs. Linux xattrs remain separate acceptance.
if not hasattr(os,'listxattr'):os.listxattr=lambda *args,**kwargs:[]
root=pathlib.Path(os.environ['ANKER_LOCAL_ROOT']);r=json.load(sys.stdin);op=r['operation']
assert 'StrictHostKeyChecking=yes' in sys.argv
assert r['version']==1
h.authorize(op,sys.argv[-1].endswith('--read-only'))
with (root/'calls').open('a') as log: log.write(op+'\n')
def probe(root=root,test_mode=False):
 hashes={};metadata={}
 for p in sorted((root/'etc').rglob('*')):
  if not p.is_file():continue
  rel=p.relative_to(root).as_posix();e=h.entry_meta(p,rel)
  hashes[rel]=h.digest(p.read_bytes());metadata[rel]={k:e[k] for k in ('type','mode','uid','gid','xattrs')}
 return {'hostname':'local-proof','pve_version':'8.4','interfaces':[],'disks':[],'details':{'file_hashes':hashes,'file_metadata':metadata,'users':{str(os.getuid()):'tester'},'groups':{str(os.getgid()):'testers'},'capabilities':{'restore_protocol':2,'remove_files':True,'journal':True,'recovery_read_protocol':1},'command_results':{k:{'state':'ok'} for k in ('pve_version','file_inventory','users','groups')}}}
h.probe=probe
try:
 if op=='probe':
  inv=probe()
  if (root/'old-helper').exists():inv['details']['capabilities'].pop('recovery_read_protocol')
  print(json.dumps(inv))
 elif op=='collect':
  db=root/'var/lib/pve-cluster/config.db';db.parent.mkdir(parents=True,exist_ok=True)
  if not db.exists():
   with sqlite3.connect(db) as connection:connection.execute('CREATE TABLE tree(inode INTEGER PRIMARY KEY,parent INTEGER,version INTEGER,writer INTEGER,mtime INTEGER,type INTEGER,name TEXT,data BLOB)')
  with tempfile.TemporaryDirectory() as tmp:
   out=pathlib.Path(tmp);c=h.collect(root,out,['/etc/sysctl.d'],test_mode=True);(out/'collection.json').write_text(json.dumps(c))
   with tarfile.open(fileobj=sys.stdout.buffer,mode='w|') as archive:
    for p in sorted(out.rglob('*')):archive.add(p,arcname=p.relative_to(out).as_posix(),recursive=False)
 elif op=='read-recovery':
  if (root/'old-helper').exists():raise ValueError('unknown operation')
  print(json.dumps(h.read_recovery_files(root,r['paths'],r['storage_ids'],['/etc/sysctl.d'])))
 elif op=='apply':
  assert r['confirm']==r['plan_id']
  # The transport fixture keeps the helper's pre-write inventory race check.
  expected=dict(r['expected_inventory']);expected.pop('fingerprint',None);expected.pop('captured_at',None)
  current=probe()
  def norm(v):
   if isinstance(v,dict):return {k:norm(x) for k,x in v.items() if x not in ('',None,[],{},False)}
   if isinstance(v,list):return [norm(x) for x in v]
   return v
  assert norm(expected)==norm(current),'target inventory changed'
  result=h.apply_files(root,r['files'],test_mode=True,plan_id=r['plan_id'])
  if (root/'lose-reply').exists():sys.exit(1)
  print(json.dumps(result))
 elif op=='restore-status':
  if (root/'pause-status').exists():
   (root/'status-entered').touch()
   while (root/'pause-status').exists():time.sleep(0.01)
  if (root/'read-error').exists():raise ValueError('journal read failed')
  print(json.dumps(h.restore_status(root,r['plan_id'])))
 elif op=='rollback':print(json.dumps(h.rollback_files(root,r['plan_id'],r['confirm'],test_mode=True)))
 else:raise ValueError('unknown operation')
except Exception as exc:
 print(json.dumps(getattr(exc,'result',{'state':'failed','operation_id':'','error':str(exc)})));sys.exit(1)
`
	if err = os.WriteFile(filepath.Join(dir, "driver.py"), []byte(driver), 0600); err != nil {
		t.Fatal(err)
	}
	// Constant script; all paths are environment values, never evaluated as code.
	if err = os.WriteFile(filepath.Join(dir, "ssh"), []byte("#!/bin/sh\nexec \"$ANKER_LOCAL_PYTHON\" \"$ANKER_LOCAL_DRIVER\" \"$@\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANKER_LOCAL_PYTHON", python)
	t.Setenv("ANKER_LOCAL_DRIVER", filepath.Join(dir, "driver.py"))
	t.Setenv("ANKER_LOCAL_HELPER", helper)
	t.Setenv("ANKER_LOCAL_ROOT", root)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	s := testService(t)
	s.Collector = SSHCollector{}
	h, _ := s.Host("host1")
	for _, name := range []string{"backup-key", "restore-key", "known"} {
		if err = os.WriteFile(filepath.Join(dir, name), []byte("local fixture identity"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	h.KeyPath = filepath.Join(dir, "backup-key")
	h.KnownHostsPath = filepath.Join(dir, "known")
	h.RestoreKeyPath = filepath.Join(dir, "restore-key")
	h.RestoreSSHUser = "anker-restore"
	if err = s.SaveHost(h); err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateBackup(t.Context(), h.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Warnings) > 0 {
		t.Fatalf("fixture collection warnings: %v", b.Warnings)
	}
	if err = os.WriteFile(filepath.Join(root, path), []byte("vm.swappiness=60\n"), 0640); err != nil {
		t.Fatal(err)
	}
	return s, PlanRequest{BackupID: b.ID, TargetID: h.ID, Scenario: "files", Files: []string{path}, Decisions: map[string]FileDecision{path: DecisionBackup}}, root
}
func localPlan(t *testing.T, s *Service, r PlanRequest) Plan {
	t.Helper()
	i, err := s.CaptureRecovery(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	r.SnapshotID = i.SnapshotID
	p, err := s.CreatePlan(t.Context(), r)
	if err != nil || p.State != "ready" {
		t.Fatalf("plan state=%s blockers=%v steps=%+v err=%v", p.State, p.Blockers, p.Steps, err)
	}
	return p
}
func TestRecoveryTargetComparePrepareApplyReconcileRoundTrip(t *testing.T) {
	s, r, root := localRecoveryHelper(t)
	p := localPlan(t, s, r)
	c, err := s.ComparePlanFile(p.ID, r.Files[0], "target", false)
	if err != nil || c.State != "different" || len(c.Hunks) == 0 {
		t.Fatalf("comparison %+v %v", c, err)
	}
	for _, path := range []string{filepath.Join(s.Root, "plans", p.ID), filepath.Join(s.Root, "plans", p.ID, "target-snapshot.json"), filepath.Join(s.Root, "plans", p.ID, "target-files", r.Files[0])} {
		st, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		want := os.FileMode(0600)
		if st.IsDir() {
			want = 0700
		}
		if st.Mode().Perm() != want {
			t.Fatalf("private mode %s %o", path, st.Mode().Perm())
		}
	}
	// Tampered prepared content cannot reach the real helper.
	prepared := filepath.Join(s.Root, "plans", p.ID, "prepared-files", r.Files[0])
	original, _ := os.ReadFile(prepared)
	os.WriteFile(prepared, []byte("tampered"), 0600)
	if _, err = s.ApplyPlan(t.Context(), p.ID, p.ID); err == nil {
		t.Fatal("tamper accepted")
	}
	os.WriteFile(prepared, original, 0600)
	os.WriteFile(filepath.Join(root, "lose-reply"), nil, 0600)
	p, err = s.ApplyPlan(t.Context(), p.ID, p.ID)
	if err == nil || p.State != "interrupted" {
		t.Fatalf("lost reply %+v %v", p, err)
	}
	got, _ := os.ReadFile(filepath.Join(root, r.Files[0]))
	if !bytes.Equal(got, original) {
		t.Fatalf("helper did not apply %q", got)
	}
	p, err = s.ReconcilePlan(t.Context(), p.ID)
	if err != nil || p.State != "checks_pending" || p.Result.OperationID != p.ID || p.Result.RebootVerified {
		t.Fatalf("journal %+v %v", p, err)
	}
	if _, err = s.ApplyPlan(t.Context(), p.ID, p.ID); err == nil {
		t.Fatal("same plan replayed")
	}
	p, err = s.RollbackPlan(t.Context(), p.ID, p.ID)
	if err != nil || p.State != "rolled_back" {
		t.Fatalf("rollback %+v %v", p, err)
	}
	got, _ = os.ReadFile(filepath.Join(root, r.Files[0]))
	if string(got) != "vm.swappiness=60\n" {
		t.Fatalf("rollback content %q", got)
	}
}
func TestRecoveryReadyLostReplyReconcileWithLocalJournal(t *testing.T) {
	for _, kind := range []string{"not-found", "found", "read-error", "queued", "running", "connection"} {
		t.Run(kind, func(t *testing.T) {
			s, r, root := localRecoveryHelper(t)
			p := localPlan(t, s, r)
			if kind == "found" {
				os.WriteFile(filepath.Join(root, "lose-reply"), nil, 0600)
				if _, err := s.ApplyPlan(t.Context(), p.ID, p.ID); err == nil {
					t.Fatal("expected lost reply")
				}
				p.State = "ready"
				p.Result = nil
				s.savePlan(p)
			}
			if kind == "read-error" {
				os.WriteFile(filepath.Join(root, "read-error"), nil, 0600)
			}
			if kind == "queued" || kind == "running" {
				s.Store.Put("jobs", "pending", Job{ID: "pending", HostID: p.TargetID, State: kind, Kind: "restore"})
			}
			if kind == "connection" {
				h, _ := s.Host(p.TargetID)
				h.Address = "192.0.2.99"
				s.SaveHost(h)
			}
			a := NewAuth(s.Store)
			a.CreateUser("restore-integration", "long-test-password", "restore", false)
			token, _, _ := a.Login("restore-integration", "long-test-password")
			server := httptest.NewServer(Handler(s, a, false))
			defer server.Close()
			req, _ := http.NewRequest("POST", server.URL+"/api/plans/"+p.ID+"/reconcile", strings.NewReader("{}"))
			req.Header.Set("X-Anker-Request", "1")
			req.AddCookie(&http.Cookie{Name: "anker_session", Value: token})
			resp, err := server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			raw, _ := io.ReadAll(resp.Body)
			if kind == "not-found" || kind == "found" {
				if resp.StatusCode != 200 {
					t.Fatalf("reconcile %d %s", resp.StatusCode, raw)
				}
				var got Plan
				json.Unmarshal(raw, &got)
				want := "interrupted"
				if kind == "found" {
					want = "checks_pending"
				}
				if got.State != want || got.Result == nil || got.Result.RebootVerified {
					t.Fatalf("invalid resolved state %+v", got)
				}
				if _, err = s.ApplyPlan(t.Context(), p.ID, p.ID); err == nil {
					t.Fatal("resolved uncertainty permitted replay")
				}
			} else {
				if resp.StatusCode == 200 {
					t.Fatalf("unsafe reconciliation %s", raw)
				}
				got, _ := s.Plan(p.ID)
				if got.State != "ready" {
					t.Fatalf("error fabricated evidence %+v", got)
				}
			}
		})
	}
}
func TestRecoveryHelperUnavailableKeepsExport(t *testing.T) {
	s, r, root := localRecoveryHelper(t)
	os.WriteFile(filepath.Join(root, "old-helper"), nil, 0600)
	i, err := s.CaptureRecovery(t.Context(), r)
	if err != nil || i.SnapshotID != "" || i.Automatic || !strings.Contains(strings.Join(i.Warnings, " "), "Hostverbindung aktualisieren") {
		t.Fatalf("old helper boundary %+v %v", i, err)
	}
	var out bytes.Buffer
	if err = s.ExportBackup(r.BackupID, &out); err != nil || out.Len() == 0 {
		t.Fatalf("original export %v", err)
	}
}
func TestRecoveryExportWithoutRunningService(t *testing.T) {
	s, r, root := localRecoveryHelper(t)
	p := localPlan(t, s, r)
	before, _ := os.ReadFile(filepath.Join(root, "calls"))
	if err := s.Store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := OpenStore(filepath.Join(s.Root, "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	offline, err := NewService(s.Root, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	s = offline // Reopened catalog, no HTTP/socket listener and no live collector.
	var out bytes.Buffer
	if err := s.ExportPlan(p.ID, &out); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(root, "calls"))
	if !bytes.Equal(before, after) {
		t.Fatal("offline export queried host")
	}
	tr := tar.NewReader(&out)
	found := false
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Name == "original/original-files/"+r.Files[0] {
			data, _ := io.ReadAll(tr)
			if string(data) != "vm.swappiness=10\n" || h.Mode != 0640 {
				t.Fatalf("original archive lost data/metadata %+v %q", h, data)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("missing original export member")
	}
}

func TestRecoveryReadyJournalReservationExcludesQueueAndChangedConnection(t *testing.T) {
	for _, kind := range []string{"queue", "connection"} {
		t.Run(kind, func(t *testing.T) {
			s, r, root := localRecoveryHelper(t)
			p := localPlan(t, s, r)
			pause := filepath.Join(root, "pause-status")
			if err := os.WriteFile(pause, nil, 0600); err != nil {
				t.Fatal(err)
			}
			defer os.Remove(pause)
			done := make(chan error, 1)
			go func() { _, err := s.ReconcilePlan(t.Context(), p.ID); done <- err }()
			deadline := time.Now().Add(5 * time.Second)
			for {
				if _, err := os.Stat(filepath.Join(root, "status-entered")); err == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("actual helper never entered journal read")
				}
				time.Sleep(10 * time.Millisecond)
			}
			if kind == "queue" {
				if _, err := s.QueueBackup(p.TargetID); err == nil {
					t.Fatal("queued job admitted during journal read")
				}
			} else {
				h, _ := s.Host(p.TargetID)
				h.Address = "192.0.2.99"
				if err := s.SaveHost(h); err != nil {
					t.Fatal(err)
				}
			}
			os.Remove(pause)
			err := <-done
			if kind == "queue" && err != nil {
				t.Fatal(err)
			}
			if kind == "connection" && err == nil {
				t.Fatal("changed endpoint evidence accepted")
			}
		})
	}
}

func TestRecoveryAuthenticAbandonedJournalStaysInterruptedAndRollsBack(t *testing.T) {
	s, r, root := localRecoveryHelper(t)
	p := localPlan(t, s, r)
	os.WriteFile(filepath.Join(root, "lose-reply"), nil, 0600)
	p, err := s.ApplyPlan(t.Context(), p.ID, p.ID)
	if err == nil || p.State != "interrupted" {
		t.Fatalf("expected lost reply: %s %v", p.State, err)
	}
	// Model termination immediately before final journal completion. All file and
	// rollback evidence comes from the actual helper's durable application journal.
	journalPath := filepath.Join(root, "var/lib/anker-host/rollback", p.ID, "journal.json")
	raw, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	var journal map[string]any
	json.Unmarshal(raw, &journal)
	journal["state"] = "writing"
	raw, _ = json.Marshal(journal)
	if err = os.WriteFile(journalPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	p, err = s.ReconcilePlan(t.Context(), p.ID)
	if err != nil || p.State != "interrupted" || p.Result.State != "interrupted" || p.Result.RollbackPath == "" {
		t.Fatalf("abandoned journal %+v %v", p, err)
	}
	if _, err = s.ApplyPlan(t.Context(), p.ID, p.ID); err == nil {
		t.Fatal("interrupted replay accepted")
	}
	// Reload saved plan and re-read authentic journal before separate rollback.
	p, err = s.Plan(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	p, err = s.RollbackPlan(t.Context(), p.ID, p.ID)
	if err != nil || p.State != "rolled_back" {
		t.Fatalf("rollback %+v %v", p, err)
	}
	got, _ := os.ReadFile(filepath.Join(root, r.Files[0]))
	if string(got) != "vm.swappiness=60\n" {
		t.Fatalf("wrong restored original %q", got)
	}
}
