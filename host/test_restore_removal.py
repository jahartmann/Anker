import importlib.util, pathlib, tempfile, unittest
from unittest.mock import patch
spec=importlib.util.spec_from_file_location('removal_helper', pathlib.Path(__file__).with_name('anker_host.py'))
h=importlib.util.module_from_spec(spec);spec.loader.exec_module(h)

class RemovalTests(unittest.TestCase):
 def setUp(self):
  self.tmp=tempfile.TemporaryDirectory();self.addCleanup(self.tmp.cleanup);self.root=pathlib.Path(self.tmp.name)
  (self.root/'etc/sysctl.d').mkdir(parents=True);self.path=self.root/'etc/sysctl.d/extra.conf';self.path.write_bytes(b'vm.swappiness=10\n');self.path.chmod(0o640)
 def item(self):
  return {'path':'etc/sysctl.d/extra.conf','action':'remove','remove_confirmed':True,'before_sha':h.digest(self.path.read_bytes()),'expected_metadata':h.metadata(self.path,True)}
 def apply(self,items=None):return h.apply_files(self.root,items or [self.item()],True,'remove-test')
 def test_remove_and_restore_original_content_and_mode(self):
  result=self.apply();self.assertFalse(self.path.exists());self.assertEqual(result['state'],'applied')
  folder=self.root/'var/lib/anker-host/rollback/remove-test'
  self.assertEqual((folder/'before/etc/sysctl.d/extra.conf').read_bytes(),b'vm.swappiness=10\n')
  self.assertEqual(h.restore_status(self.root,'remove-test')['state'],'applied')
  result=h.rollback_files(self.root,'remove-test','remove-test',True)
  self.assertEqual(result['state'],'rolled_back');self.assertEqual(self.path.read_bytes(),b'vm.swappiness=10\n');self.assertEqual(self.path.stat().st_mode&0o777,0o640)
 def test_confirmation_and_missing_target_fail_without_deleting(self):
  item=self.item();item['remove_confirmed']=False
  with self.assertRaisesRegex(ValueError,'confirmation'):self.apply([item])
  self.assertTrue(self.path.exists());item=self.item();self.path.unlink()
  with self.assertRaises(ValueError):self.apply([item])
 def test_changed_content_and_metadata_block_removal(self):
  item=self.item();self.path.write_bytes(b'foreign')
  with self.assertRaises(ValueError):self.apply([item])
  self.path.write_bytes(b'vm.swappiness=10\n');self.path.chmod(0o600)
  with self.assertRaises(ValueError):self.apply([item])
  self.assertTrue(self.path.exists())
 def test_recreated_file_is_preserved_on_rollback(self):
  self.apply();self.path.write_bytes(b'foreign recreation')
  result=h.rollback_files(self.root,'remove-test','remove-test',True)
  self.assertEqual(result['state'],'rollback_conflict');self.assertEqual(self.path.read_bytes(),b'foreign recreation')
 def test_crash_after_unlink_can_be_recovered_without_replay(self):
  original=pathlib.Path.unlink
  def unlink(p,*args,**kwargs):
   result=original(p,*args,**kwargs)
   if p==self.path:raise KeyboardInterrupt()
   return result
  item=self.item()
  with patch.object(pathlib.Path,'unlink',unlink):
   with self.assertRaises(KeyboardInterrupt):self.apply([item])
  self.assertEqual(h.restore_status(self.root,'remove-test')['state'],'interrupted')
  with self.assertRaises(Exception):self.apply([item])
  self.assertEqual(h.rollback_files(self.root,'remove-test','remove-test',True)['state'],'rolled_back');self.assertTrue(self.path.exists())
 def test_failure_after_removal_rolls_it_back(self):
  other=self.root/'etc/sysctl.d/second.conf';other.write_bytes(b'old');second=self.item();second['path']='etc/sysctl.d/second.conf';second['before_sha']=h.digest(b'old');second['expected_metadata']=h.metadata(other,True)
  original=pathlib.Path.unlink
  def unlink(p,*args,**kwargs):
   if p==other:raise OSError('unlink failed')
   return original(p,*args,**kwargs)
  with patch.object(pathlib.Path,'unlink',unlink):
   with self.assertRaises(OSError) as cm:self.apply([self.item(),second])
  self.assertEqual(cm.exception.result['state'],'rolled_back');self.assertTrue(self.path.exists());self.assertEqual(other.read_bytes(),b'old')
 def test_protected_and_non_file_targets_are_preserved(self):
  item=self.item();item['path']='etc/hostname'
  with self.assertRaises(ValueError):self.apply([item])
  self.path.unlink();self.path.symlink_to('other')
  with self.assertRaises(ValueError):self.apply([item|{'path':'etc/sysctl.d/extra.conf'}])
 def test_rollback_backup_is_durable_before_unlink(self):
  original=pathlib.Path.unlink
  def unlink(p,*args,**kwargs):
   if p==self.path:
    folder=self.root/'var/lib/anker-host/rollback/remove-test'
    self.assertEqual((folder/'before/etc/sysctl.d/extra.conf').read_bytes(),b'vm.swappiness=10\n')
    self.assertEqual(h.load_journal(folder)['files'][0]['phase'],'writing')
   return original(p,*args,**kwargs)
  with patch.object(pathlib.Path,'unlink',unlink):self.apply()

if __name__=='__main__':unittest.main()
