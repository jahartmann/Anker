import importlib.util
import json
import pathlib
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('discovery_helper', pathlib.Path(__file__).with_name('anker_host.py'))
helper = importlib.util.module_from_spec(spec)
spec.loader.exec_module(helper)


class DiscoveryTests(unittest.TestCase):
 def setUp(self):
  temp = tempfile.TemporaryDirectory()
  self.addCleanup(temp.cleanup)
  self.root = pathlib.Path(temp.name)

 def test_standalone_does_not_query_a_cluster(self):
  with patch.object(helper, 'command', side_effect=AssertionError('standalone command')):
   self.assertEqual(helper.discovery_hints(self.root), {'version': 1, 'state': 'not_applicable', 'hints': []})
  helper.authorize('discover', read_only=True)

 def test_cluster_api_and_ring_fallback_are_bounded_and_deduplicated(self):
  config = self.root/'etc/pve'
  config.mkdir(parents=True)
  (config/'corosync.conf').write_text('nodelist {\n node {\n  name: pve-one\n  ring0_addr: 10.20.0.11\n  ring1_addr: 10.21.0.11\n }\n node {\n  name: pve-two\n  ring0_addr: unresolved.example\n }\n}\n')
  with patch.object(helper, 'command', return_value=json.dumps([{'type': 'node', 'name': 'pve-one', 'ip': '10.20.0.11'}, {'type': 'cluster', 'ip': '10.20.0.99'}])) as command:
   result = helper.discovery_hints(self.root)
  command.assert_called_once_with(['pvesh', 'get', '/cluster/status', '--output-format', 'json'])
  self.assertEqual(result['state'], 'partial')
  self.assertEqual([h['address'] for h in result['hints']], ['10.20.0.11', '10.21.0.11'])
  with patch.object(helper, 'command', return_value=''):
   self.assertEqual(len(helper.discovery_hints(self.root)['hints']), 2)

 def test_pbs_reads_remote_hosts_without_secrets_or_datastore_contents(self):
  config = self.root/'etc/proxmox-backup'
  config.mkdir(parents=True)
  (config/'remote.cfg').write_text('remote: backup-two\n host 10.20.0.22\n password sensitive-password\n auth-id secret-user@pbs\n fingerprint sensitive-fingerprint\n\nremote: dns-only\n host other.example\n')
  with patch.object(helper, 'command', side_effect=AssertionError('PBS command')), patch.object(helper, 'pbs_data_roots', side_effect=AssertionError('datastore access')):
   result = helper.discovery_hints(self.root)
  self.assertEqual(result['hints'], [{'address': '10.20.0.22', 'name': 'backup-two', 'source': 'pbs-config'}])
  self.assertEqual(result['state'], 'partial')
  self.assertNotIn('sensitive', json.dumps(result))
  self.assertNotIn('secret-user', json.dumps(result))

 def test_oversized_config_and_invalid_peer_addresses(self):
  config = self.root/'etc/proxmox-backup'
  config.mkdir(parents=True)
  path = config/'remote.cfg'
  path.write_text('remote: loopback\n host 127.0.0.1\nremote: multicast\n host 224.0.0.1\n')
  self.assertEqual(helper.discovery_hints(self.root)['hints'], [])
  path.write_text('x' * (1024*1024 + 1))
  with self.assertRaises(ValueError): helper.discovery_hints(self.root)


if __name__ == '__main__': unittest.main()
