import importlib.util
import json
import pathlib
import subprocess
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('pbs_helper', pathlib.Path(__file__).with_name('anker_host.py'))
helper = importlib.util.module_from_spec(spec)
spec.loader.exec_module(helper)

class PBSTests(unittest.TestCase):
 def setUp(self):
  self.temp = tempfile.TemporaryDirectory()
  self.addCleanup(self.temp.cleanup)
  self.root = pathlib.Path(self.temp.name)
  self.config = self.root/'etc/proxmox-backup'
  self.config.mkdir(parents=True)
  (self.config/'datastore.cfg').write_text('datastore: main\n path /etc/datastore\n')
  (self.config/'remote.cfg').write_text('remote: other\n password encoded-secret\n')
  (self.config/'sync.cfg').write_text('sync: nightly\n store main\n schedule daily\n remote remote-secret\n')
  (self.root/'etc/datastore/.chunks').mkdir(parents=True)
  (self.root/'etc/datastore/.chunks/payload').write_text('backup-data')
  (self.root/'etc/datastore/vm').mkdir()
  (self.root/'etc/datastore/vm/index').write_text('guest-index')
  (self.root/'usr/local').mkdir(parents=True)
  self.calls = []
  xattrs=patch.object(helper.os,'listxattr',return_value=[],create=True)
  xattrs.start();self.addCleanup(xattrs.stop)
 def command_run(self, args, **kwargs):
  self.calls.append(args)
  out = b''
  if args[:3] == ['dpkg-query','-W','-f=${Version}']:out=b'4.0.1-1'
  elif args[0] == 'lsblk':out=b'{"blockdevices":[]}'
  elif args[0] == 'ip':out=b'[]'
  return subprocess.CompletedProcess(args,0,out,b'')
 def probe(self):
  with patch.object(helper.subprocess,'run',side_effect=self.command_run):return helper.probe(self.root)
 def collect(self,paths=None):
  with patch.object(helper.subprocess,'run',side_effect=self.command_run):
   out=self.root/'out';out.mkdir()
   return helper.collect(self.root,out,paths or ['/etc','/usr/local'])
 def test_detection_and_inventory_exclude_pve_commands_and_data(self):
  inv=self.probe()
  self.assertEqual(inv['host_type'],'pbs')
  self.assertEqual(inv['pbs_version'],'4.0.1-1')
  self.assertEqual(inv['pve_version'],'')
  self.assertFalse(any(c[0] in ('pveversion','pvesm','qm','pct','pvecm','proxmox-boot-tool') for c in self.calls))
  self.assertFalse(any(p.startswith('etc/datastore') for p in inv['details']['file_hashes']))
  self.assertEqual(inv['pbs_datastores'],[{'name':'main','path':'/etc/datastore'}])
  self.assertEqual(inv['pbs_jobs'][0]['schedule'],'daily')
  self.assertNotIn('remote-secret',json.dumps(inv))
  self.assertNotIn('encoded-secret',json.dumps(inv))
  self.assertEqual(inv['details']['command_results']['guests']['state'],'not_applicable')
 def test_capture_is_successful_without_pmxcfs_and_contains_secrets_as_files_only(self):
  col=self.collect()
  self.assertEqual(col['warnings'],[])
  self.assertFalse((self.root/'out/recovery/config.db').exists())
  self.assertEqual((self.root/'out/files/etc/proxmox-backup/remote.cfg').read_text(),'remote: other\n password encoded-secret\n')
  self.assertFalse(any(e['path'].startswith('etc/datastore') for e in col['entries']))
  self.assertIn('/etc/proxmox-backup',col['profile']['roots'])
 def test_custom_data_root_is_rejected(self):
  with self.assertRaisesRegex(ValueError,'only permits'):self.collect(['/etc','/mnt/backup'])
 def test_unreadable_or_invalid_datastore_config_stops_before_traversal(self):
  (self.config/'datastore.cfg').write_text('datastore: main\n path /etc/datastore\n path /other\n')
  with self.assertRaisesRegex(ValueError,'duplicate'):self.collect()
 def test_empty_fresh_server_without_datastores_is_supported(self):
  (self.config/'datastore.cfg').unlink()
  inv=self.probe()
  self.assertEqual(inv['pbs_datastores'],[])
  self.assertFalse(any(p.startswith('etc/datastore') for p in inv['details']['file_hashes']))
 def test_broken_optional_job_is_reported_as_partial(self):
  (self.config/'sync.cfg').write_text('invalid job\n')
  col=self.collect()
  self.assertIn('PBS inventory query failed: pbs_sync_jobs',col['warnings'])
 def test_symlink_capture_root_cannot_bypass_data_exclusion(self):
  (self.root/'etc/alias').symlink_to(self.root/'etc/datastore',target_is_directory=True)
  with self.assertRaisesRegex(ValueError,'symlink'):self.collect(['/etc/alias/vm'])
 def test_pve_without_pbs_directory_keeps_legacy_detection(self):
  self.assertEqual(helper.host_type(self.root/'missing'),'pve')
