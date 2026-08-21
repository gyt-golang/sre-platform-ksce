#!/usr/bin/env python3
# 给 master02/03/node01/node02 补 containerd docker.io 镜像加速配置
import os, paramiko
HOSTS = ["10.0.0.41","10.0.0.107","10.0.0.136","10.0.0.242"]
NAMES = {"10.0.0.41":"master02","10.0.0.107":"master03","10.0.0.136":"node01","10.0.0.242":"node02"}
PWD = os.environ["KSCE_PWD"]
TOML = '''server = "https://registry-1.docker.io"

[host."https://docker.m.daocloud.io"]
  capabilities = ["pull", "resolve"]

[host."https://docker.1panel.live"]
  capabilities = ["pull", "resolve"]

[host."https://docker.nju.edu.cn"]
  capabilities = ["pull", "resolve"]
'''
for h in HOSTS:
    c=paramiko.SSHClient(); c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    c.connect(h,username="root",password=PWD,timeout=12,allow_agent=False,look_for_keys=False)
    def run(cmd):
        _,o,e=c.exec_command(cmd,timeout=30); o.channel.recv_exit_status()
        return (o.read().decode("utf-8","replace")+e.read().decode("utf-8","replace")).strip()
    run("mkdir -p /etc/containerd/certs.d/docker.io")
    sftp=c.open_sftp()
    import io
    sftp.putfo(io.BytesIO(TOML.encode()), "/etc/containerd/certs.d/docker.io/hosts.toml")
    sftp.close()
    # containerd config_path 模式下 hosts.toml 动态生效，无需重启
    print(f"{NAMES[h]}: hosts.toml written ->", run("head -3 /etc/containerd/certs.d/docker.io/hosts.toml").replace("\n"," | "))
    c.close()
print("\nall nodes patched (no containerd restart needed)")
