import base64
import importlib.util
import os
import pathlib
import subprocess
import tempfile
import unittest
from unittest.mock import patch

spec=importlib.util.spec_from_file_location('pbs_restore',pathlib.Path(__file__).with_name('anker_host.py'))
h=importlib.util.module_from_spec(spec);spec.loader.exec_module(h)

class PBSRestoreTests(unittest.TestCase):
 def setUp(self):
  self.tmp=tempfile.TemporaryDirectory();self.addCleanup(self.tmp.cleanup);self.root=pathlib.Path(self.tmp.name)
  self.config=self.root/'etc/proxmox-backup';self.config.mkdir(parents=True)
  (self.config/'datastore.cfg').write_text('datastore: main\n path /mnt/backup\n')
  (self.config/'remote.cfg').write_bytes(b'old-remote')
  (self.root/'mnt/backup/.chunks').mkdir(parents=True)
  (self.root/'mnt/backup/.chunks/chunk').write_bytes(b'unchanged-chunk')
  (self.root/'mnt/backup/vm/100').mkdir(parents=True)
  (self.root/'mnt/backup/vm/100/index').write_bytes(b'unchanged-index')
  (self.root/'proc/self').mkdir(parents=True)
  (self.root/'proc/self/mountinfo').write_text('1 0 8:1 / / rw - ext4 /dev/sda rw\n')
  self.active=False
  stub=patch.object(h,'pbs_service_states',side_effect=lambda:{name:'active' if self.active else 'inactive' for name in h.PBS_SERVICES});stub.start();self.addCleanup(stub.stop)
  xattrs=patch.object(h.os,'listxattr',return_value=[],create=True);xattrs.start();self.addCleanup(xattrs.stop)
 def item(self,path,data):
  p=self.root/path;before,meta=h.file_state(self.root,path,True)
  return {'path':path,'type':'file','mode':0o600,'uid':os.getuid(),'gid':os.getgid(),'before_sha':before,'expected_metadata':meta,'content':base64.b64encode(data).decode()}
 def apply(self,items):return h.apply_files(self.root,items,test_mode=True,plan_id='pbs-restore')
 def data_state(self):
  return {p.relative_to(self.root).as_posix():(p.read_bytes(),p.stat().st_ino,p.stat().st_mtime_ns) for p in (self.root/'mnt').rglob('*') if p.is_file()}
 def test_apply_and_rollback_preserve_data_and_permissions(self):
  before=self.data_state();remote=self.config/'remote.cfg';remote.chmod(0o640)
  result=self.apply([self.item('etc/proxmox-backup/remote.cfg',b'remote: new\n password protected\n')])
  self.assertEqual(result['state'],'applied');self.assertEqual(self.data_state(),before)
  self.assertEqual(remote.stat().st_mode&0o777,0o600)
  result=h.rollback_files(self.root,'pbs-restore','pbs-restore',test_mode=True)
  self.assertEqual(result['state'],'rolled_back');self.assertEqual(remote.read_bytes(),b'old-remote');self.assertEqual(remote.stat().st_mode&0o777,0o640);self.assertEqual(self.data_state(),before)
 def test_active_services_block_apply_and_rollback(self):
  item=self.item('etc/proxmox-backup/remote.cfg',b'new');self.active=True
  with self.assertRaisesRegex(ValueError,'stop proxmox'):self.apply([item])
  self.assertEqual((self.config/'remote.cfg').read_bytes(),b'old-remote')
  self.active=False;self.apply([item]);self.active=True
  with self.assertRaises(Exception):h.rollback_files(self.root,'pbs-restore','pbs-restore',test_mode=True)
  self.assertEqual((self.config/'remote.cfg').read_bytes(),b'new')
 def test_datastore_relocation_and_missing_store_are_rejected(self):
  for data in (b'datastore: main\n path /mnt/other\n',b'datastore: different\n path /mnt/backup\n',b''):
   with self.assertRaisesRegex(ValueError,'PBS datastore'):self.apply([self.item('etc/proxmox-backup/datastore.cfg',data)])
  self.assertFalse((self.root/'mnt/other').exists())
 def test_job_triggers_cannot_be_restored_raw(self):
  cases={'sync.cfg':b'sync: one\n store main\n remote-store other\n schedule daily\n','verification.cfg':b'verification: one\n store main\n schedule daily\n','prune.cfg':b'prune: one\n store main\n schedule daily\n','datastore.cfg':b'datastore: main\n path /mnt/backup\n gc-schedule daily\n'}
  for name,data in cases.items():
   with self.assertRaisesRegex(ValueError,'disabled'):self.apply([self.item('etc/proxmox-backup/'+name,data)])
 def test_prepared_jobs_are_written_without_starting_them(self):
  before=self.data_state()
  items=[self.item('etc/proxmox-backup/sync.cfg',b'sync: one\n store main\n remote-store other\n group-filter type:vm\n group-filter type:ct\n'),self.item('etc/proxmox-backup/prune.cfg',b'prune: one\n store main\n schedule daily\n disable true\n')]
  self.assertEqual(self.apply(items)['state'],'applied');self.assertEqual(self.data_state(),before)
 def test_datastore_identity_drift_blocks_rollback(self):
  self.apply([self.item('etc/proxmox-backup/remote.cfg',b'new')])
  chunks=self.root/'mnt/backup/.chunks';chunks.rename(chunks.with_name('.old-chunks'));chunks.mkdir()
  with self.assertRaises(Exception):h.rollback_files(self.root,'pbs-restore','pbs-restore',test_mode=True)
  self.assertEqual((self.config/'remote.cfg').read_bytes(),b'new')
 def test_system_files_unknown_configs_and_deletions_are_rejected(self):
  for path in ('etc/hosts','mnt/backup/vm/100/index','etc/proxmox-backup/unknown.cfg'):
   with self.assertRaises(ValueError):self.apply([self.item(path,b'bad')])
  item=self.item('etc/proxmox-backup/remote.cfg',b'new');item['action']='remove'
  with self.assertRaisesRegex(ValueError,'no deletions'):self.apply([item])
 def test_config_directory_inside_datastore_is_not_writable(self):
  (self.config/'datastore.cfg').write_text('datastore: main\n path /etc/proxmox-backup\n')
  (self.config/'.chunks').mkdir()
  with self.assertRaisesRegex(ValueError,'configuration directory overlaps'):self.apply([self.item('etc/proxmox-backup/remote.cfg',b'new')])
 def test_reader_never_lists_or_reads_guest_data(self):
  with self.assertRaises(ValueError):h.read_recovery_files(self.root,['mnt/backup/vm/100/index'],[],['/etc','/mnt/backup'])
  with self.assertRaises(ValueError):h.read_recovery_files(self.root,['etc/proxmox-backup/remote.cfg'],['main'],['/etc'])
 def test_foreign_edit_survives_rollback(self):
  self.apply([self.item('etc/proxmox-backup/remote.cfg',b'new')]);(self.config/'remote.cfg').write_bytes(b'foreign')
  result=h.rollback_files(self.root,'pbs-restore','pbs-restore',test_mode=True)
  self.assertEqual(result['state'],'rollback_conflict');self.assertEqual((self.config/'remote.cfg').read_bytes(),b'foreign')

 def test_config_bind_mount_alias_is_rejected(self):
  (self.root/'proc/self/mountinfo').write_text('1 0 8:1 / / rw - ext4 /dev/sda rw\n2 1 8:1 /mnt/backup/vm/100 /etc/proxmox-backup rw - ext4 /dev/sda rw\n')
  with self.assertRaisesRegex(ValueError,'configuration mount'):self.apply([self.item('etc/proxmox-backup/remote.cfg',b'new')])
  self.assertFalse((self.root/'var/lib/anker-host').exists())
 def test_journal_inside_datastore_is_rejected_before_lock_creation(self):
  (self.config/'datastore.cfg').write_text('datastore: main\n path /var/lib\n')
  (self.root/'var/lib/.chunks').mkdir(parents=True)
  with self.assertRaisesRegex(ValueError,'journal must be outside'):self.apply([self.item('etc/proxmox-backup/remote.cfg',b'new')])
  self.assertFalse((self.root/'var/lib/anker-host').exists())

 def test_sync_missing_remote_store_is_rejected(self):
  with self.assertRaisesRegex(ValueError,'remote-store'):self.apply([self.item('etc/proxmox-backup/sync.cfg',b'sync: one\n store main\n')])
 def test_journal_bind_mount_alias_is_rejected_before_lock(self):
  (self.root/'proc/self/mountinfo').write_text('1 0 8:1 / / rw - ext4 /dev/sda rw\n2 1 8:1 /mnt/backup /var/lib/anker-host rw - ext4 /dev/sda rw\n')
  with self.assertRaisesRegex(ValueError,'journal mount'):self.apply([self.item('etc/proxmox-backup/remote.cfg',b'new')])
  self.assertFalse((self.root/'var/lib/anker-host').exists())

 def test_dedicated_configuration_filesystem_is_supported(self):
  (self.root/'proc/self/mountinfo').write_text('1 0 8:1 / / rw - ext4 /dev/sda rw\n2 1 8:2 / /etc/proxmox-backup rw - ext4 /dev/sdb rw\n')
  self.assertEqual(self.apply([self.item('etc/proxmox-backup/remote.cfg',b'new')])['state'],'applied')
