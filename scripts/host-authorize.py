#!/usr/bin/env python3
"""Install one public Ed25519 key with Anker's fixed SSH restrictions."""
import argparse
import base64
import fcntl
import os
import pathlib
import shlex
import stat
import tempfile


def parse_key(text):
    lines=text.strip().splitlines()
    if len(lines)!=1:raise ValueError('Genau einen öffentlichen Ed25519-Schlüssel angeben')
    fields=lines[0].split()
    if len(fields)<2 or fields[0]!='ssh-ed25519':raise ValueError('Öffentliche Ed25519-Datei verwenden, keinen privaten Schlüssel oder authorized_keys-Optionen')
    try:raw=base64.b64decode(fields[1],validate=True)
    except Exception as error:raise ValueError('Ungültiger öffentlicher Schlüssel') from error
    if raw[:19]!=b'\x00\x00\x00\x0bssh-ed25519\x00\x00\x00\x20' or len(raw)!=51:raise ValueError('Ungültiger öffentlicher Ed25519-Schlüssel')
    return 'ssh-ed25519 '+base64.b64encode(raw).decode()


def install_key(path,key,read_only):
    path=pathlib.Path(path)
    key=parse_key(key)
    lock=os.open(str(path)+'.lock',os.O_CREAT|os.O_RDWR|os.O_NOFOLLOW,0o600)
    try:
        if not stat.S_ISREG(os.fstat(lock).st_mode):raise ValueError('Ungültige Schlüssel-Sperrdatei')
        fcntl.flock(lock,fcntl.LOCK_EX)
        lines=[]
        if path.is_symlink():raise ValueError('authorized_keys darf kein symbolischer Link sein')
        try:fd=os.open(path,os.O_RDONLY|os.O_NOFOLLOW)
        except FileNotFoundError:fd=None
        if fd is not None:
            with os.fdopen(fd,'r') as old:
                if not stat.S_ISREG(os.fstat(old.fileno()).st_mode) or os.fstat(old.fileno()).st_size>1024*1024:raise ValueError('Ungültige authorized_keys-Datei')
                for line in old:
                    try:fields=shlex.split(line,comments=True)
                    except ValueError:fields=[]
                    # Replace this key even if it was previously unrestricted.
                    if any(fields[i:i+2]==key.split() for i in [0,1]):continue
                    lines.append(line.rstrip('\n'))
        command='sudo -n /usr/local/lib/anker/anker-host'+(' --read-only' if read_only else '')
        lines.append('restrict,command="'+command+'" '+key+(' anker-backup' if read_only else ' anker-restore'))
        with tempfile.NamedTemporaryFile(dir=path.parent,prefix='.anker-authorize-',delete=False,mode='w') as staged:
            temporary=pathlib.Path(staged.name)
            try:
                os.fchmod(staged.fileno(),0o644)
                staged.write('\n'.join(lines)+'\n');staged.flush();os.fsync(staged.fileno())
                os.replace(temporary,path)
                directory=os.open(path.parent,os.O_RDONLY)
                try:os.fsync(directory)
                finally:os.close(directory)
            finally:temporary.unlink(missing_ok=True)
    finally:os.close(lock)


def installed_keys(path):
    try:fd=os.open(path,os.O_RDONLY|os.O_NOFOLLOW|os.O_NONBLOCK)
    except FileNotFoundError:return set()
    with os.fdopen(fd,'r') as source:
        if not stat.S_ISREG(os.fstat(source.fileno()).st_mode):raise ValueError('Ungültige authorized_keys-Datei')
        text=source.read(1024*1024+1)
        if len(text)>1024*1024:raise ValueError('authorized_keys-Datei zu groß')
    keys=set()
    for line in text.splitlines():
        try:fields=shlex.split(line,comments=True)
        except ValueError:continue
        for index in [0,1]:
            if fields[index:index+1]==['ssh-ed25519'] and len(fields)>index+1:
                keys.add(parse_key(' '.join(fields[index:index+2])))
    return keys


def install_roles(paths,keys,lock_path):
    if not keys:return
    keys={role:parse_key(key) for role,key in keys.items()}
    lock=os.open(lock_path,os.O_CREAT|os.O_RDWR|os.O_NOFOLLOW,0o600)
    try:
        if not stat.S_ISREG(os.fstat(lock).st_mode):raise ValueError('Ungültige Rollen-Sperrdatei')
        fcntl.flock(lock,fcntl.LOCK_EX)
        existing={role:installed_keys(path) for role,path in paths.items()}
        for role,key in keys.items():
            other='restore' if role=='backup' else 'backup'
            if key in existing[other] or key==keys.get(other):
                raise ValueError('Sicherung und Wiederherstellung benötigen verschiedene Schlüssel; Schlüssel ist bereits für die andere Rolle eingerichtet')
        for role,key in keys.items():install_key(paths[role],key,role=='backup')
    finally:os.close(lock)


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--backup-key')
    parser.add_argument('--restore-key')
    parser.add_argument('--validate-only',action='store_true')
    args=parser.parse_args()
    keys={}
    for role,path in [('backup',args.backup_key),('restore',args.restore_key)]:
        if path:
            file=pathlib.Path(path)
            info=file.stat()
            if not stat.S_ISREG(info.st_mode) or info.st_size>16384:raise ValueError('Öffentliche Schlüsseldatei ungültig oder zu groß')
            with file.open() as source:text=source.read(16385)
            if len(text)>16384:raise ValueError('Öffentliche Schlüsseldatei zu groß')
            keys[role]=parse_key(text)
    if keys.get('backup') and keys.get('backup')==keys.get('restore'):raise ValueError('Sicherung und Wiederherstellung benötigen verschiedene Schlüssel')
    if args.validate_only:return
    if os.geteuid()!=0:raise ValueError('Schlüssel auf dem Proxmox-Host als root einrichten')
    paths={'backup':'/var/lib/anker-ssh/.ssh/authorized_keys','restore':'/var/lib/anker-restore-ssh/.ssh/authorized_keys'}
    install_roles(paths,keys,'/var/lib/anker-host-authorize.lock')


if __name__=='__main__':
    try:main()
    except (ValueError,OSError) as error:raise SystemExit('Anker-Hostzugang: '+str(error))
