"""Exercise release packaging with bounded compiler/npm/signing fixtures."""
import os
import pathlib
import shutil
import subprocess
import tarfile
import tempfile
import unittest


class BuildReleaseTests(unittest.TestCase):
 def run_release(self, mismatch=False):
  with tempfile.TemporaryDirectory() as temporary:
   root=pathlib.Path(temporary);commands=root/'commands';commands.mkdir()
   source=pathlib.Path(__file__).resolve().parents[1]
   for directory in ['scripts','deploy','host','docs','web','internal/updater']:(root/directory).mkdir(parents=True)
   for file in ['scripts/build-release.sh','scripts/install-server.sh','scripts/install-state.py','scripts/install-release.py','scripts/verify-release.sh','scripts/install-host.sh','scripts/host-authorize.py','scripts/setup-sftp-export.sh','deploy/anker.service','deploy/anker-updater.service','deploy/anker.1','host/anker_host.py','docs/OPERATIONS.md','docs/RECOVERY.md','docs/UPDATES.md','docs/SUPPORT.md','README.md','LICENSE','CHANGELOG.md','CONTRIBUTING.md','SECURITY.md']:
    shutil.copyfile(source/file,root/file)
   (root/'internal/updater/official_public.key').write_text('trusted fixture key\n')
   (root/'scripts/release-notices.py').write_text('import pathlib,sys\npathlib.Path(sys.argv[1]).write_text("fixture dependency notice\\n")\n')
   def executable(name,body):
    path=commands/name;path.write_text('#!/bin/sh\n'+body);path.chmod(0o755)
   executable('go','''echo "go $*" >>"$ANKER_TEST_LOG"
if [ "$1" = build ]; then
 while [ "$#" -gt 0 ]; do
  if [ "$1" = -o ]; then shift; printf 'fixture executable\\n' >"$1"; exit 0; fi
  shift
 done
 exit 2
fi
shift
while [ "$#" -gt 0 ]; do
 case "$1" in -mode) shift; mode=$1;; -dir) shift; output=$1;; esac
 shift
done
if [ "${mode:-}" = public ]; then
 mkdir -p "$output"
 printf '%s\\n' "$ANKER_TEST_PUBLIC_KEY" >"$output/public.key"
 printf 'fixture public pem\\n' >"$output/public.pem"
fi
''')
   executable('npm','echo "npm $*" >>"$ANKER_TEST_LOG"\n')
   for name in ['mkdir','cp','rm','tar','cmp']:os.symlink(shutil.which(name),commands/name)
   os.symlink(shutil.which('python3'),commands/'python3')
   log=root/'log';env=dict(os.environ,PATH=str(commands),ANKER_TEST_LOG=str(log),ANKER_TEST_PUBLIC_KEY='other fixture key' if mismatch else 'trusted fixture key')
   result=subprocess.run(['/bin/sh','scripts/build-release.sh','v1.2.3','dist'],cwd=root,env=env,text=True,capture_output=True)
   if not mismatch:
    self.assertEqual(result.returncode,0,result.stdout+result.stderr)
    for arch in ['amd64','arm64']:
     with tarfile.open(root/'dist'/('anker-linux-'+arch+'.tar.gz')) as package:
      self.assertEqual(package.extractfile('./deploy/anker.1').read(),(source/'deploy/anker.1').read_bytes())
      self.assertEqual(package.extractfile('./LICENSE').read(),(source/'LICENSE').read_bytes())
      self.assertEqual(package.extractfile('./THIRD_PARTY_NOTICES.txt').read(),b'fixture dependency notice\n')
   else:
    self.assertFalse(list((root/'dist').glob('*.tar.gz')),'mismatched key produced a release package')
   return result,log.read_text()

 def test_both_architecture_packages_include_manual_and_licenses(self):
  self.run_release()

 def test_different_signing_key_aborts_before_frontend_or_binary_build(self):
  result,log=self.run_release(mismatch=True)
  self.assertNotEqual(result.returncode,0)
  self.assertIn('Schlüssel',result.stderr)
  self.assertNotIn('npm ',log);self.assertNotIn('go build',log)


if __name__=='__main__':unittest.main()
