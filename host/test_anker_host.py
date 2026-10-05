import unittest,tempfile,pathlib,sqlite3,importlib.util,json
spec=importlib.util.spec_from_file_location('helper',pathlib.Path(__file__).with_name('anker_host.py'))
helper=importlib.util.module_from_spec(spec)
spec.loader.exec_module(helper)
class HostHelperTests(unittest.TestCase):
 def test_snapshot_matches_database_tree(self):
  with tempfile.TemporaryDirectory() as d:
   root=pathlib.Path(d);(root/'etc').mkdir();(root/'var/lib/pve-cluster').mkdir(parents=True)
   db=sqlite3.connect(root/'var/lib/pve-cluster/config.db')
   db.execute('CREATE TABLE tree(inode INTEGER PRIMARY KEY,parent INTEGER,version INTEGER,writer INTEGER,mtime INTEGER,type INTEGER,name TEXT,data BLOB)')
   db.execute('INSERT INTO tree VALUES(0,0,1,0,1,4,?,NULL)',('',))
   db.execute('INSERT INTO tree VALUES(1,0,1,0,1,8,?,?)',('storage.cfg',b'dir: local\n path /var/lib/vz\n'));db.commit();db.close()
   out=root/'out';out.mkdir();result=helper.collect(root,out,['/etc'],test_mode=True)
   self.assertEqual((out/'files/etc/pve/storage.cfg').read_bytes(),b'dir: local\n path /var/lib/vz\n')
   self.assertTrue((out/'recovery/config.db').is_file())
 def test_symlink_is_not_followed(self):
  with tempfile.TemporaryDirectory() as d:
   r=pathlib.Path(d);(r/'etc').mkdir();(r/'etc/escape').symlink_to('/etc/passwd');out=r/'out';out.mkdir()
   c=helper.collect(r,out,['/etc'],test_mode=True)
   e=next(e for e in c['entries'] if e['path']=='etc/escape')
   self.assertEqual(e['type'],'symlink');self.assertFalse((out/'files/etc/escape').is_symlink())
 def test_missing_pve_is_partial(self):
  with tempfile.TemporaryDirectory() as d:
   r=pathlib.Path(d);(r/'etc').mkdir();out=r/'out';out.mkdir()
   c=helper.collect(r,out,['/etc'],test_mode=True)
   self.assertTrue(any('config.db' in w for w in c['warnings']))
 def test_apply_rejects_drift_before_write(self):
  with tempfile.TemporaryDirectory() as d:
   r=pathlib.Path(d);(r/'etc').mkdir();(r/'etc/test').write_text('changed')
   with self.assertRaises(ValueError):helper.apply_files(r,[{'path':'etc/test','before_sha':'wrong','content':'bmV3','mode':420,'uid':0,'gid':0}],test_mode=True)
   self.assertEqual((r/'etc/test').read_text(),'changed')
 def test_traversal_rejected(self):
  with tempfile.TemporaryDirectory() as d:
   with self.assertRaises(ValueError):helper.apply_files(pathlib.Path(d),[{'path':'../escape','content':'eA=='}],test_mode=True)
if __name__=='__main__':unittest.main()
