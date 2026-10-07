import base64, importlib.util, json, os, pathlib, tempfile, unittest
from unittest.mock import patch
spec=importlib.util.spec_from_file_location('restore_helper', pathlib.Path(__file__).with_name('anker_host.py'))
h=importlib.util.module_from_spec(spec);spec.loader.exec_module(h)

class RestoreHardeningTests(unittest.TestCase):
 def setUp(self):
  self.tmp=tempfile.TemporaryDirectory();self.addCleanup(self.tmp.cleanup);self.root=pathlib.Path(self.tmp.name);(self.root/'etc').mkdir()
  self.a=self.root/'etc/a';self.b=self.root/'etc/b';self.a.write_bytes(b'old-a');self.b.write_bytes(b'old-b');self.a.chmod(0o644);self.b.chmod(0o644)
 def item(self,p,new):
  st=p.stat()
  return {'path':p.relative_to(self.root).as_posix(),'before_sha':h.digest(p.read_bytes()),'content':base64.b64encode(new).decode(),'type':'file','mode':0o640,'uid':st.st_uid,'gid':st.st_gid,'expected_metadata':{'type':'file','mode':st.st_mode&0o7777,'uid':st.st_uid,'gid':st.st_gid,'xattrs':{}}}
 def apply(self,items,id='plan-test'):
  return h.apply_files(self.root,items,test_mode=True,plan_id=id)
 def test_second_write_error_preserves_cause_and_rolls_back(self):
  original=h.os.replace
  def replace(src,dest):
   if pathlib.Path(dest)==self.b:raise OSError('disk write failed')
   return original(src,dest)
  with patch.object(h.os,'replace',side_effect=replace):
   with self.assertRaisesRegex(OSError,'disk write failed') as cm:self.apply([self.item(self.a,b'new-a'),self.item(self.b,b'new-b')])
  self.assertEqual(self.a.read_bytes(),b'old-a');self.assertEqual(self.b.read_bytes(),b'old-b')
  self.assertEqual(cm.exception.result['state'],'rolled_back')
 def test_concurrent_content_edit_before_first_replace_is_preserved(self):
  original=h.sync_directory;changed=False
  def sync(p):
   nonlocal changed
   original(p)
   if not changed and 'rollback' in pathlib.Path(p).parts:self.a.write_bytes(b'admin-edit');changed=True
  with patch.object(h,'sync_directory',side_effect=sync):
   with self.assertRaises(Exception):self.apply([self.item(self.a,b'new-a')])
  self.assertEqual(self.a.read_bytes(),b'admin-edit')
 def test_same_content_metadata_drift_is_preserved(self):
  item=self.item(self.a,b'new-a');self.a.chmod(0o600)
  with self.assertRaises(Exception):self.apply([item])
  self.assertEqual(self.a.read_bytes(),b'old-a');self.assertEqual(self.a.stat().st_mode&0o777,0o600)
 def test_external_edit_after_first_write_prevents_rollback(self):
  original=h.os.replace
  def replace(src,dest):
   if pathlib.Path(dest)==self.b:self.a.write_bytes(b'admin-after');raise OSError('second failed')
   return original(src,dest)
  with patch.object(h.os,'replace',side_effect=replace):
   with self.assertRaises(OSError) as cm:self.apply([self.item(self.a,b'new-a'),self.item(self.b,b'new-b')])
  self.assertEqual(self.a.read_bytes(),b'admin-after');self.assertEqual(cm.exception.result['state'],'rollback_conflict')
 def test_duplicate_operation_never_applies_again(self):
  items=[self.item(self.a,b'new-a')];r=self.apply(items);self.a.write_bytes(b'admin-after')
  with self.assertRaises(Exception):self.apply(items)
  self.assertEqual(self.a.read_bytes(),b'admin-after');self.assertEqual(r['operation_id'],'plan-test')
 def test_crash_is_inspectable_and_cannot_be_replayed(self):
  original=h.os.replace
  def replace(src,dest):
   value=original(src,dest)
   if pathlib.Path(dest)==self.a:raise KeyboardInterrupt()
   return value
  items=[self.item(self.a,b'new-a')]
  with patch.object(h.os,'replace',side_effect=replace):
   with self.assertRaises(KeyboardInterrupt):self.apply(items)
  r=h.restore_status(self.root,'plan-test');self.assertEqual(r['state'],'interrupted');self.assertEqual(self.a.read_bytes(),b'new-a')
  with self.assertRaises(Exception):self.apply(items)
  r=h.rollback_files(self.root,'plan-test','plan-test',test_mode=True);self.assertEqual(r['state'],'rolled_back');self.assertEqual(self.a.read_bytes(),b'old-a')
 def test_explicit_rollback_restores_original_mode(self):
  self.a.chmod(0o600);self.apply([self.item(self.a,b'new-a')]);r=h.rollback_files(self.root,'plan-test','plan-test',test_mode=True)
  self.assertEqual(r['state'],'rolled_back');self.assertEqual(self.a.read_bytes(),b'old-a');self.assertEqual(self.a.stat().st_mode&0o777,0o600)
 def test_invalid_operation_id_cannot_escape_journal(self):
  with self.assertRaises(ValueError):self.apply([self.item(self.a,b'new-a')],'../outside')
  self.assertEqual(self.a.read_bytes(),b'old-a')
 def test_network_and_protected_files_are_manual(self):
  for rel in ('etc/network/interfaces','etc/hostname','etc/corosync/corosync.conf','etc/ssh/ssh_host_rsa_key','etc/systemd/system/test.service'):
   p=self.root/rel;p.parent.mkdir(parents=True,exist_ok=True);p.write_bytes(b'old')
   with self.assertRaises(ValueError):self.apply([self.item(p,b'new')],rel.replace('/','-'))
   self.assertEqual(p.read_bytes(),b'old')
 def test_target_xattrs_cannot_be_silently_removed(self):
  with patch.object(h.os,'listxattr',return_value=['user.test'],create=True),patch.object(h.os,'getxattr',return_value=b'value',create=True):
   with self.assertRaises(Exception):self.apply([self.item(self.a,b'new')])
  self.assertEqual(self.a.read_bytes(),b'old-a')
 def test_read_authority_cannot_inspect_or_rollback(self):
  for operation in ('restore-status','rollback'):
   with self.assertRaisesRegex(ValueError,'read-only'):h.authorize(operation,True)


class InventoryHardeningTests(unittest.TestCase):
 def test_failed_required_commands_remain_distinguishable_from_empty_inventory(self):
  import subprocess
  with tempfile.TemporaryDirectory() as d,patch.object(h.subprocess,'run',side_effect=OSError('cannot execute')):
   r=h.probe(pathlib.Path(d));details=r['details']
  self.assertEqual(details['capabilities'],{'restore_protocol':2,'journal':True})
  for key in ('pve_version','interfaces','disks','addresses','routes','packages','storage'):
   self.assertEqual(details['command_results'][key]['state'],'failed')
   self.assertIn('error',details['command_results'][key])
 def test_probe_records_target_metadata_accounts_and_physical_interface(self):
  import subprocess
  with tempfile.TemporaryDirectory() as d:
   root=pathlib.Path(d);(root/'etc').mkdir();p=root/'etc/config';p.write_bytes(b'config');p.chmod(0o640)
   (root/'sys/class/net/eno1/device').mkdir(parents=True)
   def command(args,**kw):
    raw=b''
    if args[:4]==['ip','-j','link','show']:raw=b'[{"ifname":"eno1","address":"aa"},{"ifname":"vmbr0","address":"bb"}]'
    if args[0]=='lsblk':raw=b'{"blockdevices":[{"name":"sda","type":"disk","size":100,"children":[{"name":"sda1","type":"part","fstype":"ext4","size":50}]}]}'
    return subprocess.CompletedProcess(args,0,raw,b'')
   with patch.object(h.subprocess,'run',side_effect=command):r=h.probe(root)
   self.assertEqual(r['interfaces'][0]['physical'],True);self.assertEqual(r['interfaces'][1]['physical'],False);self.assertEqual(r['interfaces'][1]['pci'],'')
   self.assertEqual(r['disks'][1]['parent'],'sda');self.assertEqual(r['disks'][1]['filesystem'],'ext4')
   m=r['details']['file_metadata']['etc/config'];self.assertEqual(m['mode'],0o640);self.assertEqual(m['type'],'file')
   self.assertTrue(r['details']['users']);self.assertTrue(r['details']['groups'])
 def test_file_walk_error_is_reported(self):
  import subprocess
  def walk(path,**kw):kw['onerror'](PermissionError('walk denied'));return iter([])
  with tempfile.TemporaryDirectory() as d:
   root=pathlib.Path(d);(root/'etc').mkdir()
   with patch.object(h.os,'walk',side_effect=walk),patch.object(h.subprocess,'run',return_value=subprocess.CompletedProcess([],0,b'',b'')):r=h.probe(root)
  self.assertEqual(r['details']['command_results']['file_inventory']['state'],'failed')
 def test_malformed_required_command_json_is_failed(self):
  import subprocess
  with tempfile.TemporaryDirectory() as d,patch.object(h.subprocess,'run',return_value=subprocess.CompletedProcess([],0,b'not-json',b'')):
   r=h.probe(pathlib.Path(d))
  self.assertEqual(r['details']['command_results']['interfaces']['state'],'failed')
  self.assertEqual(r['details']['command_results']['disks']['state'],'failed')

class RestoreProtocolTests(unittest.TestCase):
 setUp=RestoreHardeningTests.setUp
 item=RestoreHardeningTests.item
 apply=RestoreHardeningTests.apply
 def test_owner_identity_mismatch_blocks_write(self):
  item=self.item(self.a,b'new');item.update(user='unexpected-owner',group='unexpected-group')
  with patch.object(h.os,'listxattr',return_value=[],create=True):
   with self.assertRaisesRegex(ValueError,'owner identity'):h.apply_files(self.root,[item],plan_id='owner-check')
  self.assertEqual(self.a.read_bytes(),b'old-a')
 def test_simultaneous_helper_cannot_write(self):
  with h.mutation_lock(self.root):
   with self.assertRaises(BlockingIOError):self.apply([self.item(self.a,b'new')])
  self.assertEqual(self.a.read_bytes(),b'old-a')
 def test_mode_drift_during_staging_blocks_replace(self):
  original=h.sync_directory;changed=False
  def sync(p):
   nonlocal changed
   original(p)
   if not changed and 'rollback' in pathlib.Path(p).parts:self.a.chmod(0o600);changed=True
  with patch.object(h,'sync_directory',side_effect=sync):
   with self.assertRaises(Exception):self.apply([self.item(self.a,b'new')])
  self.assertEqual(self.a.read_bytes(),b'old-a');self.assertEqual(self.a.stat().st_mode&0o777,0o600)
 def test_regular_file_required(self):
  (self.root/'etc/dir').mkdir()
  with self.assertRaises(ValueError):self.apply([{'path':'etc/dir','before_sha':'missing','content':'bmV3'}])
 def test_source_xattrs_requires_manual_restore(self):
  item=self.item(self.a,b'new');item['xattrs']={'user.test':'eA=='}
  with self.assertRaises(ValueError):self.apply([item])
  self.assertEqual(self.a.read_bytes(),b'old-a')
 def test_failed_operation_id_is_never_reexecuted(self):
  original=h.write_file
  with patch.object(h,'write_file',side_effect=OSError('no space')):
   with self.assertRaises(OSError):self.apply([self.item(self.a,b'new')])
  self.assertEqual(h.restore_status(self.root,'plan-test')['state'],'failed')
  with self.assertRaises(OSError):self.apply([self.item(self.a,b'new')])
  self.assertEqual(self.a.read_bytes(),b'old-a')
 def test_restore_main_status_and_rollback_require_confirmation(self):
  import io
  self.apply([self.item(self.a,b'new')])
  def request(value):
   stdin=type('Stdin',(),{'buffer':io.BytesIO(json.dumps(value).encode()+b'\n')})();stdout=io.StringIO()
   with patch.object(h.os,'geteuid',return_value=0),patch.object(h.sys,'platform','linux'),patch.object(h.sys,'argv',['anker-host']),patch.object(h.sys,'stdin',stdin),patch.object(h.sys,'stdout',stdout),patch.object(h.os,'listxattr',return_value=[],create=True):h.main(root=self.root)
   return json.loads(stdout.getvalue())
  r=request({'version':1,'operation':'restore-status','plan_id':'plan-test'});self.assertEqual(r['state'],'applied')
  with self.assertRaisesRegex(ValueError,'confirmation'):request({'version':1,'operation':'rollback','plan_id':'plan-test','confirm':'wrong'})
  r=request({'version':1,'operation':'rollback','plan_id':'plan-test','confirm':'plan-test'});self.assertEqual(r['state'],'rolled_back')

class RecoveryEvidenceTests(unittest.TestCase):
 setUp=RestoreHardeningTests.setUp
 item=RestoreHardeningTests.item
 apply=RestoreHardeningTests.apply
 def test_status_exposes_ambiguous_file_phase_after_crash(self):
  original=h.os.replace
  def replace(src,dest):
   value=original(src,dest)
   if pathlib.Path(dest)==self.a:raise KeyboardInterrupt()
   return value
  with patch.object(h.os,'replace',side_effect=replace):
   with self.assertRaises(KeyboardInterrupt):self.apply([self.item(self.a,b'new')])
  r=h.restore_status(self.root,'plan-test');self.assertTrue(any('etc/a' in check and 'writing' in check for check in r['checks']))
 def test_empty_explicit_operation_id_is_invalid(self):
  with self.assertRaises(ValueError):self.apply([self.item(self.a,b'new')],'')
 def test_external_symlink_does_not_prevent_other_owned_files_rollback(self):
  original=h.os.replace
  def replace(src,dest):
   if pathlib.Path(dest)==self.b:
    self.a.unlink();self.a.symlink_to('foreign');raise OSError('second write failed')
   return original(src,dest)
  c=self.root/'etc/c';c.write_bytes(b'old-c')
  with patch.object(h.os,'replace',side_effect=replace):
   with self.assertRaises(OSError) as cm:self.apply([self.item(c,b'new-c'),self.item(self.a,b'new-a'),self.item(self.b,b'new-b')])
  self.assertTrue(self.a.is_symlink());self.assertEqual(c.read_bytes(),b'old-c');self.assertEqual(cm.exception.result['state'],'rollback_conflict')
 def test_cleanup_error_does_not_mask_original_write_error(self):
  original_replace=h.os.replace;original_unlink=h.os.unlink
  def replace(src,dest):
   if pathlib.Path(dest)==self.b:raise OSError('original disk error')
   return original_replace(src,dest)
  def unlink(path,*args,**kw):
   if pathlib.Path(path).name.startswith('.anker-'):raise OSError('cleanup error')
   return original_unlink(path,*args,**kw)
  with patch.object(h.os,'replace',side_effect=replace),patch.object(h.os,'unlink',side_effect=unlink):
   with self.assertRaisesRegex(OSError,'original disk error'):self.apply([self.item(self.a,b'new-a'),self.item(self.b,b'new-b')])



class PendingOperationTests(unittest.TestCase):
 setUp=RestoreHardeningTests.setUp
 item=RestoreHardeningTests.item
 apply=RestoreHardeningTests.apply
 def test_new_operation_id_is_blocked_by_interrupted_operation(self):
  original=h.os.replace
  def replace(src,dest):
   value=original(src,dest)
   if pathlib.Path(dest)==self.a:raise KeyboardInterrupt()
   return value
  with patch.object(h.os,'replace',side_effect=replace):
   with self.assertRaises(KeyboardInterrupt):self.apply([self.item(self.a,b'new-a')])
  with self.assertRaisesRegex(OSError,'unresolved'):self.apply([self.item(self.b,b'new-b')],'new-plan')
  self.assertEqual(self.b.read_bytes(),b'old-b')
  self.assertEqual(h.rollback_files(self.root,'plan-test','plan-test',test_mode=True)['state'],'rolled_back')
  self.assertEqual(self.apply([self.item(self.b,b'new-b')],'new-plan')['state'],'applied')
 def test_corrupt_previous_journal_blocks_new_operation(self):
  self.apply([self.item(self.a,b'new-a')]);folder=self.root/'var/lib/anker-host/rollback/plan-test'
  (folder/'journal.json').write_text('{bad-json')
  with self.assertRaises(OSError):self.apply([self.item(self.b,b'new-b')],'new-plan')
  self.assertEqual(self.b.read_bytes(),b'old-b')



class ExplicitRollbackResultTests(unittest.TestCase):
 setUp=RestoreHardeningTests.setUp
 item=RestoreHardeningTests.item
 apply=RestoreHardeningTests.apply
 def fail_second(self,foreign=False):
  original=h.os.replace
  def replace(src,dest):
   if pathlib.Path(dest)==self.b:
    if foreign:self.a.write_bytes(b'foreign-change')
    raise OSError('earlier disk error')
   return original(src,dest)
  with patch.object(h.os,'replace',side_effect=replace):
   with self.assertRaisesRegex(OSError,'earlier disk error') as cm:self.apply([self.item(self.a,b'new-a'),self.item(self.b,b'new-b')])
  return cm.exception.result
 def test_successful_explicit_rollback_reports_prior_error_as_history(self):
  r=self.fail_second();self.assertEqual(r['error'],'earlier disk error')
  r=h.rollback_files(self.root,'plan-test','plan-test',test_mode=True)
  self.assertEqual(r['state'],'rolled_back');self.assertNotIn('error',r)
  self.assertTrue(any('earlier disk error' in check for check in r['checks']))
  self.assertEqual(self.a.read_bytes(),b'old-a')
 def test_explicit_rollback_retries_conflict_only_after_own_after_state_is_restored(self):
  r=self.fail_second(True);self.assertEqual(r['state'],'rollback_conflict')
  r=h.rollback_files(self.root,'plan-test','plan-test',test_mode=True)
  self.assertEqual(r['state'],'rollback_conflict');self.assertEqual(self.a.read_bytes(),b'foreign-change')
  self.a.write_bytes(b'new-a')
  r=h.rollback_files(self.root,'plan-test','plan-test',test_mode=True)
  self.assertEqual(r['state'],'rolled_back');self.assertNotIn('error',r);self.assertEqual(self.a.read_bytes(),b'old-a')
 def test_explicit_rollback_accepts_conflict_already_restored_to_original(self):
  self.fail_second(True);self.a.write_bytes(b'old-a');self.a.chmod(0o644)
  r=h.rollback_files(self.root,'plan-test','plan-test',test_mode=True)
  self.assertEqual(r['state'],'rolled_back');self.assertNotIn('error',r);self.assertEqual(self.a.read_bytes(),b'old-a')



class StagingMetadataTests(unittest.TestCase):
 setUp=RestoreHardeningTests.setUp
 item=RestoreHardeningTests.item
 apply=RestoreHardeningTests.apply
 def attrs(self,path,**kwargs):
  return ['system.posix_acl_access'] if pathlib.Path(path).name.startswith('.anker-') else []
 def test_inherited_staging_acl_blocks_apply_before_target_write(self):
  with patch.object(h.os,'listxattr',side_effect=self.attrs,create=True),patch.object(h.os,'getxattr',return_value=b'inherited-extra-reader',create=True):
   with self.assertRaisesRegex(OSError,'staging.*metadata'):self.apply([self.item(self.a,b'new')])
  self.assertEqual(self.a.read_bytes(),b'old-a')
 def test_inherited_staging_acl_blocks_rollback_before_target_write(self):
  self.apply([self.item(self.a,b'new')])
  with patch.object(h.os,'listxattr',side_effect=self.attrs,create=True),patch.object(h.os,'getxattr',return_value=b'inherited-extra-reader',create=True):
   r=h.rollback_files(self.root,'plan-test','plan-test',test_mode=True)
  self.assertEqual(r['state'],'rollback_conflict');self.assertEqual(self.a.read_bytes(),b'new')

class MissingJournalStatusTests(unittest.TestCase):
 def test_absent_operation_has_bound_not_found_status(self):
  with tempfile.TemporaryDirectory() as d:r=h.restore_status(pathlib.Path(d),'cancelled-before-start')
  self.assertEqual(r['state'],'not_found');self.assertEqual(r['operation_id'],'cancelled-before-start');self.assertEqual(r['applied'],[])
 def test_existing_directory_without_journal_remains_interrupted(self):
  with tempfile.TemporaryDirectory() as d:
   root=pathlib.Path(d);(root/'var/lib/anker-host/rollback/partial').mkdir(parents=True)
   with self.assertRaises(OSError) as cm:h.restore_status(root,'partial')
  self.assertEqual(cm.exception.result['state'],'interrupted');self.assertEqual(cm.exception.result['operation_id'],'partial')



class RecoveryAuthorityFileTests(unittest.TestCase):
 setUp=RestoreHardeningTests.setUp
 item=RestoreHardeningTests.item
 apply=RestoreHardeningTests.apply
 def test_helper_and_restore_access_files_remain_manual(self):
  paths=('usr/local/lib/anker/anker-host','etc/sudoers.d/anker-restore','etc/sudoers','etc/ssh/sshd_config','etc/ssh/authorized_keys/anker-restore','etc/pam.d/sshd','etc/nsswitch.conf','etc/anker-host.json')
  for i,rel in enumerate(paths):
   with self.subTest(path=rel):
    p=self.root/rel;p.parent.mkdir(parents=True,exist_ok=True);p.write_bytes(b'current-authority')
    with self.assertRaisesRegex(ValueError,'protected'):self.apply([self.item(p,b'replaced-authority')],'protected-'+str(i))
    self.assertEqual(p.read_bytes(),b'current-authority')

if __name__=='__main__':unittest.main()
