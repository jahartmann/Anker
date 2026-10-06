#!/usr/bin/env python3
"""Durable marker and atomic final commit for the initial server install."""
import os
import pathlib
import stat
import sys

def sync_dir(path):
 fd=os.open(path,os.O_RDONLY)
 try:os.fsync(fd)
 finally:os.close(fd)

def main():
 if os.geteuid()!=0:raise ValueError('Root erforderlich')
 directory=pathlib.Path('/etc/anker');marker=directory/'install-pending'
 if sys.argv[1]=='begin':
  directory.mkdir(mode=0o700,parents=True,exist_ok=True)
  info=directory.lstat()
  if not stat.S_ISDIR(info.st_mode) or info.st_uid!=0 or info.st_mode&0o022:raise ValueError('/etc/anker muss root gehören und geschützt sein')
  try:
   fd=os.open(marker,os.O_CREAT|os.O_EXCL|os.O_WRONLY|os.O_NOFOLLOW,0o600)
  except FileExistsError:
   info=marker.lstat()
   if not stat.S_ISREG(info.st_mode) or info.st_uid!=0 or info.st_mode&0o077:raise ValueError('Installationsmarker ungültig')
  else:
   try:os.write(fd,b'Anker initial install\n');os.fsync(fd)
   finally:os.close(fd)
  sync_dir(directory);sync_dir(directory.parent)
 elif sys.argv[1]=='finish':
  staged=pathlib.Path(sys.argv[2])
  if staged.parent!=pathlib.Path('/usr/local/bin') or not staged.name.startswith('.anker-install-'):raise ValueError('Ungültiger Stagingpfad')
  files=[staged,pathlib.Path('/usr/local/libexec/anker-updater'),pathlib.Path('/etc/systemd/system/anker.service'),pathlib.Path('/etc/systemd/system/anker-updater.service'),directory/'service.env']
  if (directory/'release-public.key').exists():files.append(directory/'release-public.key')
  for path in files:
   fd=os.open(path,os.O_RDONLY|os.O_NOFOLLOW)
   try:
    info=os.fstat(fd)
    if not stat.S_ISREG(info.st_mode) or info.st_uid!=0 or info.st_mode&0o022:raise ValueError('Installationsdatei ungeschützt')
    os.fsync(fd)
   finally:os.close(fd)
  for path in {p.parent for p in files}|{directory,pathlib.Path('/srv/anker'),pathlib.Path('/srv'),pathlib.Path('/usr/local'),pathlib.Path('/etc/systemd')}:
   sync_dir(path)
  os.replace(staged,'/usr/local/bin/anker');sync_dir('/usr/local/bin')
  marker.unlink();sync_dir(directory)
 else:raise ValueError('begin oder finish verwenden')

if __name__=='__main__':
 try:main()
 except (ValueError,OSError,IndexError) as error:raise SystemExit('Installationsstatus: '+str(error))
