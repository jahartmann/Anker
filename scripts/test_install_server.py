"""Installer entry flow; systemd and real replacements are checked in root CI."""
import os
import pathlib
import shutil
import subprocess
import tempfile
import unittest

class InstallServerTests(unittest.TestCase):
 def run_installer(self,fail=False,missing_go=False):
  with tempfile.TemporaryDirectory() as temporary:
   root=pathlib.Path(temporary);repo=root/'repo';commands=root/'commands';commands.mkdir();(repo/'scripts').mkdir(parents=True)
   paths={'/usr/local/bin/anker':root/'installed'/'anker','/etc/anker':root/'etc','/run/anker-install.lock':root/'run'/'install.lock','/run/systemd/system':root/'systemd'}
   (root/'installed').mkdir();(root/'run').mkdir();(root/'systemd').mkdir();(root/'etc').mkdir()
   (root/'installed'/'anker').write_text('old executable');(repo/'go.mod').write_text('module fixture\n')
   script=pathlib.Path(__file__).with_name('install-server.sh').read_text()
   for source,target in paths.items():script=script.replace(source,str(target))
   (repo/'scripts'/'install-server.sh').write_text(script)
   # A stale output must never bypass the requested source build.
   (repo/'bin').mkdir();(repo/'bin'/'anker-linux-amd64').write_text('stale build')
   def executable(name,text):
    path=commands/name;path.write_text('#!/bin/sh\n'+text);path.chmod(0o755)
   for name in ['dirname','mktemp','rm','cp','grep']:
    os.symlink(shutil.which(name),commands/name)
   for name in ['useradd','runuser','install','ssh-keygen','python3','flock','openssl','rsync']:
    executable(name,'exit 0\n')
   executable('id','echo 0\n');executable('uname','case "$1" in -s) echo Linux;; -m) echo x86_64;; esac\n')
   executable('systemctl','echo "systemctl $*" >>"$ANKER_TEST_LOG"\nexit 0\n')
   executable('dpkg-query','echo "install ok installed"\n')
   go='''echo "go $* $CGO_ENABLED $GOTOOLCHAIN" >>"$ANKER_TEST_LOG"
[ "${ANKER_TEST_FAIL:-}" != 1 ] || exit 8
while [ "$#" -gt 0 ]; do
 if [ "$1" = -o ]; then shift; target=$1; break; fi
 shift
done
cat >"$target" <<'BINARY'
#!/bin/sh
echo "candidate $*" >>"$ANKER_TEST_LOG"
case "$1" in version) echo 'Anker fixture';; install-local) exit 0;; *) exit 3;; esac
BINARY
chmod 755 "$target"
'''
   for name in ['cat','chmod']:os.symlink(shutil.which(name),commands/name)
   template=root/'go-template';template.write_text('#!/bin/sh\n'+go);template.chmod(0o755)
   if not missing_go:shutil.copyfile(template,commands/'go');(commands/'go').chmod(0o755)
   executable('apt-get','echo "apt $*" >>"$ANKER_TEST_LOG"\nif [ "$1" = install ]; then cp "$ANKER_TEST_GO" "'+str(commands/'go')+'"; chmod 755 "'+str(commands/'go')+'"; fi\n')
   log=root/'log';env=dict(os.environ,PATH=str(commands),ANKER_TEST_LOG=str(log),ANKER_TEST_GO=str(template),ANKER_TEST_FAIL='1' if fail else '0')
   result=subprocess.run(['/bin/sh',str(repo/'scripts'/'install-server.sh'),'--no-setup'],env=env,text=True,capture_output=True)
   recorded=log.read_text() if log.exists() else ''
   self.assertEqual((root/'installed'/'anker').read_text(),'old executable')
   self.assertFalse(list(repo.glob('.anker-build-*')),'build staging not cleaned')
   return result,recorded

 def test_source_install_builds_and_updates_existing_without_manual_flags(self):
  result,log=self.run_installer()
  self.assertEqual(result.returncode,0,result.stdout+result.stderr)
  self.assertIn('go build -trimpath',log);self.assertIn('0 auto',log)
  self.assertIn('candidate install-local',log)
  self.assertNotIn('apt ',log)

 def test_failed_build_never_touches_running_installation(self):
  result,log=self.run_installer(fail=True)
  self.assertNotEqual(result.returncode,0)
  self.assertNotIn('install-local',log);self.assertNotIn('systemctl ',log)

 def test_missing_compiler_is_installed_automatically(self):
  result,log=self.run_installer(missing_go=True)
  self.assertEqual(result.returncode,0,result.stdout+result.stderr)
  self.assertIn('apt update',log);self.assertIn('apt install -y --no-install-recommends golang-go',log)
  self.assertIn('candidate install-local',log)

if __name__=='__main__':unittest.main()
