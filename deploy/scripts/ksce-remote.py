#!/usr/bin/env python3
"""金山云集群远程辅助：在 master01 上执行命令、上传文件/目录。
用法：
  KSCE_PWD=... python ksce-remote.py exec "<shell cmd>"
  KSCE_PWD=... python ksce-remote.py upload <local_path> <remote_path>
环境变量：KSCE_HOST(默认10.0.0.182) KSCE_USER(默认root) KSCE_PWD
"""
import os, sys, stat, posixpath, paramiko

HOST = os.environ.get("KSCE_HOST", "10.0.0.182")
USER = os.environ.get("KSCE_USER", "root")
PWD = os.environ["KSCE_PWD"]

def connect():
    c = paramiko.SSHClient()
    c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    c.connect(HOST, username=USER, password=PWD, timeout=15,
              allow_agent=False, look_for_keys=False)
    return c

def exec_cmd(c, cmd):
    _, o, e = c.exec_command(cmd, timeout=600, get_pty=False)
    rc = o.channel.recv_exit_status()
    out = o.read().decode("utf-8", "replace")
    err = e.read().decode("utf-8", "replace")
    if out: sys.stdout.write(out)
    if err: sys.stderr.write(err)
    print(f"\n[exit={rc}]")
    return rc

def upload(c, local, remote):
    sftp = c.open_sftp()
    local = local.replace("\\", "/")
    if os.path.isfile(local):
        # 确保远程父目录存在
        mkdir_p(sftp, posixpath.dirname(remote))
        sftp.put(local, remote)
        print(f"uploaded file {local} -> {remote}")
    else:
        mkdir_p(sftp, remote)
        for root, dirs, files in os.walk(local):
            rel = os.path.relpath(root, local).replace("\\", "/")
            rdir = remote if rel == "." else f"{remote}/{rel}"
            mkdir_p(sftp, rdir)
            for f in files:
                sftp.put(os.path.join(root, f), f"{rdir}/{f}")
        print(f"uploaded dir {local} -> {remote}")
    sftp.close()

def mkdir_p(sftp, path):
    if not path or path == ".": return
    try:
        sftp.stat(path); return
    except IOError:
        mkdir_p(sftp, posixpath.dirname(path))
        try: sftp.mkdir(path)
        except IOError: pass

if __name__ == "__main__":
    c = connect()
    try:
        if sys.argv[1] == "exec":
            sys.exit(exec_cmd(c, sys.argv[2]))
        elif sys.argv[1] == "upload":
            upload(c, sys.argv[2], sys.argv[3])
        else:
            print("unknown subcmd"); sys.exit(2)
    finally:
        c.close()
