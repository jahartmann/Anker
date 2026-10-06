import base64
import importlib.util
import pathlib
import tempfile
import unittest

spec=importlib.util.spec_from_file_location('host_authorize',pathlib.Path(__file__).with_name('host-authorize.py'))
authorize=importlib.util.module_from_spec(spec)
spec.loader.exec_module(authorize)

def public_key(fill):
    algorithm=b'ssh-ed25519'
    raw=len(algorithm).to_bytes(4,'big')+algorithm+(32).to_bytes(4,'big')+bytes([fill])*32
    return 'ssh-ed25519 '+base64.b64encode(raw).decode()

class HostAuthorizeTests(unittest.TestCase):
    def test_only_one_real_public_key_is_accepted(self):
        self.assertEqual(authorize.parse_key(public_key(1)+' anker-backup'),public_key(1))
        for invalid in ['',public_key(1)+'\n'+public_key(2),'command="sh" '+public_key(1),'-----BEGIN OPENSSH PRIVATE KEY-----','ssh-ed25519 '+base64.b64encode(b'broken').decode()]:
            with self.subTest(key=invalid),self.assertRaises(ValueError):authorize.parse_key(invalid)

    def test_install_is_restricted_repeatable_and_preserves_other_keys(self):
        with tempfile.TemporaryDirectory() as temp:
            path=pathlib.Path(temp)/'authorized_keys'
            path.write_text(public_key(2)+' other\n'+public_key(1)+' old-unrestricted\n')
            authorize.install_key(path,public_key(1),True)
            authorize.install_key(path,public_key(1),True)
            lines=path.read_text().splitlines()
            self.assertEqual(len(lines),2)
            self.assertIn(public_key(2)+' other',lines)
            self.assertIn('restrict,command="sudo -n /usr/local/lib/anker/anker-host --read-only" '+public_key(1)+' anker-backup',lines)
            self.assertEqual(path.stat().st_mode&0o777,0o644)

    def test_restore_key_is_restricted_and_links_are_rejected(self):
        with tempfile.TemporaryDirectory() as temp:
            root=pathlib.Path(temp);path=root/'authorized_keys'
            authorize.install_key(path,public_key(1),False)
            self.assertEqual(path.read_text(),'restrict,command="sudo -n /usr/local/lib/anker/anker-host" '+public_key(1)+' anker-restore\n')
            target=root/'untouched';target.write_text('keep')
            path.unlink();path.symlink_to(target)
            with self.assertRaises(ValueError):authorize.install_key(path,public_key(2),True)
            self.assertEqual(target.read_text(),'keep')

    def test_later_role_change_cannot_reuse_the_other_roles_key(self):
        with tempfile.TemporaryDirectory() as temp:
            root=pathlib.Path(temp)
            paths={role:root/role for role in ['backup','restore']}
            lock=root/'roles.lock'
            authorize.install_roles(paths,{'backup':public_key(1),'restore':public_key(2)},lock)
            original={role:path.read_bytes() for role,path in paths.items()}
            for role,fill in [('restore',1),('backup',2)]:
                with self.assertRaises(ValueError):authorize.install_roles(paths,{role:public_key(fill)},lock)
                self.assertEqual({role:path.read_bytes() for role,path in paths.items()},original)
            authorize.install_roles(paths,{'restore':public_key(3)},lock)
            self.assertIn(public_key(3),paths['restore'].read_text())

if __name__=='__main__':unittest.main()
