"""Installer entry flow; systemd and real replacements are checked in root CI."""
import os
import pathlib
import shutil
import subprocess
import tempfile
import unittest


class InstallServerTests(unittest.TestCase):
 def run_installer(self, fail=False, missing_go=False, missing_man=False, fresh=False):
  with tempfile.TemporaryDirectory() as temporary:
   root=pathlib.Path(temporary);repo=root/'repo';commands=root/'commands';commands.mkdir();(repo/'scripts').mkdir(parents=True)
   paths={'/usr/local/share/man/man1':root/'man'/'man1','/usr/local/bin':root/'installed','/usr/local/libexec':root/'libexec','/etc/systemd/system':root/'units','/srv/anker':root/'data','/etc/anker':root/'etc','/run/anker-install.lock':root/'run'/'install.lock','/run/systemd/system':root/'systemd'}
   for directory in ['installed','run','systemd','etc','units']: (root/directory).mkdir()
   if not fresh: (root/'installed'/'anker').write_text('old executable')
   (repo/'go.mod').write_text('module fixture\n')
   script=pathlib.Path(__file__).with_name('install-server.sh').read_text()
   for source,target in paths.items():script=script.replace(source,str(target))
   (repo/'scripts'/'install-server.sh').write_text(script)
   (repo/'deploy').mkdir()
   manual=pathlib.Path(__file__).resolve().parents[1]/'deploy'/'anker.1'
   (repo/'deploy'/'anker.1').write_text(manual.read_text() if manual.exists() else '.TH ANKER 1\n.SH NAME\nfixture manual\n')
   for unit in ['anker.service','anker-updater.service']:(repo/'deploy'/unit).write_text('fixture '+unit+'\n')
   # A stale output must never bypass the requested source build.
   (repo/'bin').mkdir();(repo/'bin'/'anker-linux-amd64').write_text('stale build')
   def executable(name,text):
    path=commands/name;path.write_text('#!/bin/sh\n'+text);path.chmod(0o755)
   for name in ['dirname','mktemp','rm','cp','grep','cat','chmod','mv']:
    os.symlink(shutil.which(name),commands/name)
   for name in ['useradd','runuser','ssh-keygen','flock','openssl','rsync','chown']:
    executable(name,'exit 0\n')
   # Copy real files and modes; strip Linux-only ownership from the fixture.
   executable('install','''ANKER_DIRECTORY=
if [ "${1:-}" = -d ]; then ANKER_DIRECTORY=-d; shift; fi
while [ "$#" -gt 0 ]; do
 case "$1" in -o|-g) shift; shift;; *) break;; esac
done
exec '''+shutil.which('install')+''' ${ANKER_DIRECTORY} "$@"
''')
   # The state helper itself is exercised by Linux CI. Here keep its publication
   # boundary so the fresh-install path still installs an actual executable.
   executable('python3','''echo "state $*" >>"$ANKER_TEST_LOG"
if [ "${2:-}" = finish ]; then mv "$3" "'''+str(root/'installed'/'anker')+'''"; fi
''')
   if not missing_man:executable('man','exit 0\n')
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
   template=root/'go-template';template.write_text('#!/bin/sh\n'+go);template.chmod(0o755)
   if not missing_go:shutil.copyfile(template,commands/'go');(commands/'go').chmod(0o755)
   executable('apt-get','echo "apt $*" >>"$ANKER_TEST_LOG"\nif [ "$1" = install ]; then cp "$ANKER_TEST_GO" "'+str(commands/'go')+'"; chmod 755 "'+str(commands/'go')+'"; fi\n')
   log=root/'log';env=dict(os.environ,PATH=str(commands),ANKER_TEST_LOG=str(log),ANKER_TEST_GO=str(template),ANKER_TEST_FAIL='1' if fail else '0')
   result=subprocess.run(['/bin/sh',str(repo/'scripts'/'install-server.sh'),'--no-setup'],env=env,text=True,capture_output=True)
   recorded=log.read_text() if log.exists() else ''
   if not fresh:self.assertEqual((root/'installed'/'anker').read_text(),'old executable')
   self.assertFalse(list(repo.glob('.anker-build-*')),'build staging not cleaned')
   installed_manual=root/'man'/'man1'/'anker.1'
   if result.returncode == 0:
    self.assertTrue(installed_manual.is_file(),'manual missing from installation')
    self.assertEqual(installed_manual.read_bytes(),(repo/'deploy'/'anker.1').read_bytes())
    self.assertEqual(installed_manual.stat().st_mode & 0o777,0o644,'manual unreadable under private umask')
    self.assertEqual(installed_manual.parent.stat().st_mode & 0o777,0o755)
   if fail:self.assertFalse(installed_manual.exists(),'failed build changed manual')
   return result,recorded

 def test_source_install_builds_and_updates_existing_without_manual_flags(self):
  result,log=self.run_installer()
  self.assertEqual(result.returncode,0,result.stdout+result.stderr)
  self.assertIn('go build -trimpath',log);self.assertIn('0 auto',log)
  self.assertIn('candidate install-local',log)
  self.assertNotIn('apt ',log)

 def test_first_install_includes_readable_manual(self):
  result,log=self.run_installer(fresh=True)
  self.assertEqual(result.returncode,0,result.stdout+result.stderr)
  self.assertIn('state ',log);self.assertNotIn('candidate install-local',log)

 def test_failed_build_never_touches_running_installation(self):
  result,log=self.run_installer(fail=True)
  self.assertNotEqual(result.returncode,0)
  self.assertNotIn('install-local',log);self.assertNotIn('systemctl ',log)

 def test_missing_compiler_is_installed_automatically(self):
  result,log=self.run_installer(missing_go=True)
  self.assertEqual(result.returncode,0,result.stdout+result.stderr)
  self.assertIn('apt update',log);self.assertIn('apt install -y --no-install-recommends golang-go',log)
  self.assertIn('candidate install-local',log)

 def test_missing_man_is_installed_automatically(self):
  result,log=self.run_installer(missing_man=True)
  self.assertEqual(result.returncode,0,result.stdout+result.stderr)
  self.assertIn('apt update',log);self.assertIn('apt install -y --no-install-recommends man-db',log)


if __name__=='__main__':unittest.main()
