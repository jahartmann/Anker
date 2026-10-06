import base64
import hashlib
import importlib.util
import io
import json
import pathlib
import subprocess
import tarfile
import tempfile
import unittest
import urllib.request

spec=importlib.util.spec_from_file_location('install_release',pathlib.Path(__file__).with_name('install-release.py'))
installer=importlib.util.module_from_spec(spec)
spec.loader.exec_module(installer)

class InstallerTests(unittest.TestCase):
 def test_redirect_drops_token_and_rejects_http(self):
  handler=installer.SafeRedirect()
  req=urllib.request.Request('https://api.github.com/repos/example/anker',headers={'Authorization':'Bearer private-fixture'})
  redirected=handler.redirect_request(req,None,302,'redirect',{},'https://release-assets.githubusercontent.com/a')
  self.assertIsNone(redirected.get_header('Authorization'))
  with self.assertRaises(ValueError): handler.redirect_request(req,None,302,'redirect',{},'http://example.com/a')

 def test_extract_rejects_links_and_parent_escape(self):
  for name,kind in [('../escape',tarfile.REGTYPE),('/absolute',tarfile.REGTYPE),('symlink',tarfile.SYMTYPE),('hardlink',tarfile.LNKTYPE)]:
   with self.subTest(name=name),tempfile.TemporaryDirectory() as temp:
    root=pathlib.Path(temp);package=root/'package.tar.gz'
    with tarfile.open(package,'w:gz') as archive:
     member=tarfile.TarInfo(name);member.type=kind;member.linkname='../../outside';archive.addfile(member)
    with self.assertRaises(ValueError):installer.extract_package(package,root/'destination')

 def test_signed_package_verifies_before_extraction(self):
  with tempfile.TemporaryDirectory() as temp:
   root=pathlib.Path(temp);private=root/'private.pem';public=root/'public.pem'
   subprocess.run(['openssl','genpkey','-algorithm','ED25519','-out',str(private)],check=True,capture_output=True)
   subprocess.run(['openssl','pkey','-in',str(private),'-pubout','-out',str(public)],check=True,capture_output=True)
   package=root/'anker-linux-amd64.tar.gz'
   with tarfile.open(package,'w:gz') as archive:
    content=b'fixture';member=tarfile.TarInfo('./anker');member.size=len(content);member.mode=0o755;archive.addfile(member,io.BytesIO(content))
   manifest={'format':1,'version':'v0.2.0','assets':[{'name':package.name,'kind':'bundle','os':'linux','arch':'amd64','size':package.stat().st_size,'sha256':hashlib.sha256(package.read_bytes()).hexdigest()}]}
   raw=json.dumps(manifest).encode();(root/'release.json').write_bytes(raw)
   subprocess.run(['openssl','pkeyutl','-sign','-inkey',str(private),'-rawin','-in',str(root/'release.json'),'-out',str(root/'sig')],check=True,capture_output=True)
   signature=base64.b64encode((root/'sig').read_bytes())
   installer.verify_package(public,raw,signature,package,'v0.2.0','amd64')
   package.write_bytes(b'tampered')
   with self.assertRaises(ValueError):installer.verify_package(public,raw,signature,package,'v0.2.0','amd64')
   with self.assertRaises(ValueError):installer.verify_package(public,raw,signature,package,'v0.3.0','amd64')
   with self.assertRaises(ValueError):installer.verify_package(public,raw+b' ',signature,package,'v0.2.0','amd64')

if __name__=='__main__':unittest.main()
