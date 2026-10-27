package anker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Extends the existing fixed SSH fixture. Only Linux commands and service state
// are simulated; the PBS probe, collector, readers, journal and writer are real.
const pbsLocalDriver = `import importlib.util,json,os,pathlib,sys,tarfile,tempfile,subprocess
spec=importlib.util.spec_from_file_location('h',os.environ['ANKER_LOCAL_HELPER']);h=importlib.util.module_from_spec(spec);spec.loader.exec_module(h)
if not hasattr(os,'listxattr'):os.listxattr=lambda *args,**kwargs:[]
root=pathlib.Path(os.environ['ANKER_LOCAL_ROOT']);r=json.load(sys.stdin);op=r['operation']
assert 'StrictHostKeyChecking=yes' in sys.argv
h.authorize(op,sys.argv[-1].endswith('--read-only'))
with (root/'calls').open('a') as log:log.write(op+'\n')
def command(args,**kwargs):
 out=b''
 if args[:3]==['dpkg-query','-W','-f=${Version}']:out=b'4.0.1-1'
 elif args[0]=='lsblk':out=b'{"blockdevices":[]}'
 elif args[0]=='ip':out=b'[]'
 elif args[0]=='systemctl':
  assert args[1:4]==['show','--property=ActiveState','--value']
  out=b'active' if (root/'service-active').exists() else b'inactive'
 elif args[0] not in ('dpkg-query','apt-mark','zpool','vgs','lspci'):raise ValueError('unexpected command '+str(args))
 return subprocess.CompletedProcess(args,0,out,b'')
h.subprocess.run=command
try:
 if op=='probe':print(json.dumps(h.probe(root)))
 elif op=='collect':
  with tempfile.TemporaryDirectory() as tmp:
   out=pathlib.Path(tmp);c=h.collect(root,out,['/etc']);(out/'collection.json').write_text(json.dumps(c))
   with tarfile.open(fileobj=sys.stdout.buffer,mode='w|') as archive:
    for p in sorted(out.rglob('*')):archive.add(p,arcname=p.relative_to(out).as_posix(),recursive=False)
 elif op=='read-recovery':print(json.dumps(h.read_recovery_files(root,r['paths'],r['storage_ids'],['/etc'])))
 elif op=='apply':
  assert r['confirm']==r['plan_id']
  print(json.dumps(h.apply_files(root,r['files'],test_mode=True,plan_id=r['plan_id'])))
 elif op=='restore-status':print(json.dumps(h.restore_status(root,r['plan_id'])))
 elif op=='rollback':print(json.dumps(h.rollback_files(root,r['plan_id'],r['confirm'],test_mode=True)))
 else:raise ValueError('unknown operation')
except Exception as exc:
 print(json.dumps(getattr(exc,'result',{'state':'failed','operation_id':'','error':str(exc)})));sys.exit(1)
`

func pbsLocalFixture(t *testing.T, root string) {
	t.Helper()
	for _, dir := range []string{"etc/proxmox-backup", "proc/self", "mnt/source/.chunks", "mnt/target/.chunks", "mnt/target/vm/100"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for path, content := range map[string]string{
		"etc/proxmox-backup/datastore.cfg": "datastore: main\n path /mnt/source\n gc-schedule daily\n",
		"etc/proxmox-backup/remote.cfg":    "remote: old\n host original.example\n password original-secret\n",
		"etc/proxmox-backup/sync.cfg":      "sync: nightly\n store main\n remote old\n remote-store other\n schedule daily\n run-on-mount true\n",
		"etc/machine-id":                   "1234567890abcdef1234567890abcdef",
		"proc/self/mountinfo":              "1 0 8:1 / / rw - ext4 /dev/sda rw\n",
		"mnt/target/.chunks/payload":       "unchanged-chunk",
		"mnt/target/vm/100/index":          "unchanged-index",
	} {
		if err := os.WriteFile(filepath.Join(root, path), []byte(content), 0640); err != nil {
			t.Fatal(err)
		}
	}
}
func TestPBSGuidedRestoreThroughRealHelperPreservesDatastoreAndRollback(t *testing.T) {
	s, request, root := localRecoveryHelper(t, true)
	request.Files = []string{pbsConfigRoot + "remote.cfg", pbsConfigRoot + "datastore.cfg", pbsConfigRoot + "sync.cfg"}
	request.Decisions = map[string]FileDecision{}
	for _, path := range request.Files {
		request.Decisions[path] = DecisionPrepared
	}
	dataFiles := []string{"mnt/target/.chunks/payload", "mnt/target/vm/100/index"}
	before := map[string]string{}
	for _, path := range dataFiles {
		data, _ := os.ReadFile(filepath.Join(root, path))
		before[path] = string(data)
	}
	p := localPlan(t, s, request)
	data, err := os.ReadFile(filepath.Join(s.Root, "plans", p.ID, "prepared-files", pbsConfigRoot+"datastore.cfg"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "/mnt/target") || strings.Contains(string(data), "/mnt/source") || strings.Contains(string(data), "gc-schedule") {
		t.Fatal(string(data))
	}
	preview, err := s.ComparePlanFile(p.ID, pbsConfigRoot+"remote.cfg", "target", true)
	if err != nil || preview.Masked || !preview.ContentChanged {
		t.Fatal(preview, err)
	}
	// A service restart between plan and apply must stop before any writes.
	os.WriteFile(filepath.Join(root, "service-active"), nil, 0600)
	if _, err := s.ApplyPlan(t.Context(), p.ID, p.ID); err == nil {
		t.Fatal("PBS apply accepted running services")
	}
	os.Remove(filepath.Join(root, "service-active"))
	applied, err := s.ApplyPlan(t.Context(), p.ID, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if applied.Result == nil || applied.Result.State != "applied" {
		t.Fatal(applied.State, applied.Result)
	}
	syncData, _ := os.ReadFile(filepath.Join(root, pbsConfigRoot+"sync.cfg"))
	if strings.Contains(string(syncData), "schedule") || strings.Contains(string(syncData), "run-on-mount") {
		t.Fatal(string(syncData))
	}
	for _, path := range dataFiles {
		data, _ := os.ReadFile(filepath.Join(root, path))
		if string(data) != before[path] {
			t.Fatal("datastore modified", path)
		}
	}
	rolled, err := s.RollbackPlan(t.Context(), p.ID, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rolled.Result == nil || rolled.Result.State != "rolled_back" {
		t.Fatal(rolled)
	}
	remote, _ := os.ReadFile(filepath.Join(root, pbsConfigRoot+"remote.cfg"))
	if string(remote) != "target-remote\n" {
		t.Fatal(string(remote))
	}
	config, _ := os.ReadFile(filepath.Join(root, pbsConfigRoot+"datastore.cfg"))
	if string(config) != "datastore: main\n path /mnt/target\n gc-schedule hourly\n" {
		t.Fatal(string(config))
	}
	for _, path := range dataFiles {
		data, _ := os.ReadFile(filepath.Join(root, path))
		if string(data) != before[path] {
			t.Fatal("rollback changed datastore", path)
		}
	}
}
