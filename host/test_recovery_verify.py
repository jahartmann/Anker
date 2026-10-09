import io, json, subprocess, importlib.util, pathlib, tempfile, unittest
from unittest.mock import patch
spec=importlib.util.spec_from_file_location('verify_helper',pathlib.Path(__file__).with_name('anker_host.py'))
h=importlib.util.module_from_spec(spec);spec.loader.exec_module(h)
class VerificationTests(unittest.TestCase):
 def test_read_only_fixed_operation(self):
  self.assertTrue(callable(getattr(h,'verify_recovery',None)), 'fixed verification missing')
  h.authorize('verify-recovery',True)
  with tempfile.TemporaryDirectory() as d:
   root=pathlib.Path(d);(root/'etc').mkdir();(root/'etc/machine-id').write_text('a'*32+'\n');p=root/'proc/sys/kernel/random';p.mkdir(parents=True);(p/'boot_id').write_text('12345678-1234-1234-1234-123456789abc\n')
   with patch.object(h,'verify_service',return_value={'state':'ok','value':'active'}):
    v=h.verify_recovery(root,[],[])
   self.assertEqual(v['protocol'],1);self.assertEqual(v['identity']['state'],'ok');self.assertEqual(v['identity']['value'],h.digest(b'a'*32));self.assertEqual(v['boot']['value'],'12345678-1234-1234-1234-123456789abc')
   self.assertEqual(v['services'],{});self.assertEqual(v['storage_volumes'],{})
 def test_fixed_selection_validation(self):
  self.assertTrue(callable(getattr(h,'verify_recovery',None)), 'fixed verification missing')
  for services,ids in [(['sshd'],[]),(['pveproxy','pveproxy'],[]),([],['-x']),([],['a']*33)]:
   with self.subTest(services=services,ids=ids),self.assertRaises(ValueError):h.verify_recovery(pathlib.Path('/nonexistent'),services,ids)

 def test_relevant_volumes_have_deadline_and_fixed_command(self):
  with tempfile.TemporaryDirectory() as d:
   with patch.object(h,'recovery_storage_volumes',return_value={'state':'ok','volumes':['local:vm-100-disk-0']}) as read:
    v=h.verify_recovery(pathlib.Path(d),[],['local'])
   self.assertEqual(len(read.call_args.args),2,'storage query needs bounded deadline')
   self.assertEqual(v['storage_volumes']['local']['volumes'],['local:vm-100-disk-0'])

 def test_missing_identity_and_changed_boot_are_not_success(self):
  with tempfile.TemporaryDirectory() as d:
   root=pathlib.Path(d);self.assertEqual(h.verification_identity(root),{'state':'unknown','value':''})
   (root/'etc').mkdir()
   for raw in ('','0'*32,'A'*32,'a'*33,'a'*500):
    (root/'etc/machine-id').write_text(raw);self.assertEqual(h.verification_identity(root)['state'],'unknown')
   with patch.object(h,'verification_boot',side_effect=[{'state':'ok','value':'before'},{'state':'ok','value':'after'}]):
    with self.assertRaisesRegex(ValueError,'changed'):h.verify_recovery(root,[],[])
 def test_service_failures_and_fixed_command(self):
  for code,want in [(0,'ok'),(3,'failed')]:
   with patch.object(h.subprocess,'run',return_value=subprocess.CompletedProcess([],code)) as run:
    self.assertEqual(h.verify_service('pveproxy')['state'],want)
   self.assertEqual(run.call_args.args[0],['systemctl','is-active','--quiet','pveproxy'])
   self.assertEqual(run.call_args.kwargs['stdout'],subprocess.DEVNULL)
   self.assertEqual(run.call_args.kwargs['timeout'],10)
  for exc,want in [(FileNotFoundError(),'unknown'),(subprocess.TimeoutExpired('systemctl',10),'failed')]:
   with patch.object(h.subprocess,'run',side_effect=exc):self.assertEqual(h.verify_service('pvedaemon')['state'],want)
 def test_real_readonly_dispatch_rejects_arbitrary_fields(self):
  with tempfile.TemporaryDirectory() as d:
   request={'version':1,'operation':'verify-recovery','services':[],'storage_ids':[]}
   def execute(value):
    stdin=type('Stdin',(),{'buffer':io.BytesIO(json.dumps(value).encode()+b'\n')})();stdout=io.StringIO()
    with patch.object(h.os,'geteuid',return_value=0),patch.object(h.sys,'platform','linux'),patch.object(h.sys,'argv',['anker-host','--read-only']),patch.object(h.sys,'stdin',stdin),patch.object(h.sys,'stdout',stdout):h.main(root=pathlib.Path(d))
    return json.loads(stdout.getvalue())
   self.assertEqual(execute(request)['identity']['state'],'unknown')
   with self.assertRaises(ValueError):execute(dict(request,command='reboot'))
 def test_probe_captures_matching_positive_identity_and_boot_baseline(self):
  with tempfile.TemporaryDirectory() as d:
   root=pathlib.Path(d);(root/'etc').mkdir();(root/'etc/machine-id').write_text('a'*32+'\n');p=root/'proc/sys/kernel/random';p.mkdir(parents=True);(p/'boot_id').write_text('12345678-1234-1234-1234-123456789abc\n')
   with patch.object(h.subprocess,'run',side_effect=OSError('unavailable')):v=h.probe(root)
   self.assertEqual(v['details']['machine_identity_sha256'],h.digest(b'a'*32));self.assertEqual(v['details']['command_results']['machine_identity']['state'],'ok');self.assertEqual(v['details']['command_results']['boot_id']['state'],'ok')
   self.assertEqual(v['details']['capabilities']['recovery_verify_protocol'],1)
