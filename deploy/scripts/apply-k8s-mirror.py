#!/usr/bin/env python3
# 给 5 节点补 registry.k8s.io 镜像加速（k8s 官方镜像）
import os, io, paramiko
HOSTS = ["10.0.0.182","10.0.0.41","10.0.0.107","10.0.0.136","10.0.0.242"]
NAMES = {"10.0.0.182":"master01","10.0.0.41":"master02","10.0.0.107":"master03","10.0.0.136":"node01","10.0.0.242":"node02"}
PWD = os.environ["KSCE_PWD"]
TOML = '''server = "https://registry.k8s.io"

[host."https://k8s.m.daocloud.io"]
  capabilities = ["pull", "resolve"]
'''
for h in HOSTS:
    c=paramiko.SSHClient(); c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    c.connect(h,username="root",password=PWD,timeout=12,allow_agent=False,look_for_keys=False)
    def run(cmd):
        _,o,e=c.exec_command(cmd,timeout=30); o.channel.recv_exit_status()
        return (o.read().decode("utf-8","replace")+e.read().decode("utf-8","replace")).strip()
    run("mkdir -p /etc/containerd/certs.d/registry.k8s.io")
    sftp=c.open_sftp(); sftp.putfo(io.BytesIO(TOML.encode()), "/etc/containerd/certs.d/registry.k8s.io/hosts.toml"); sftp.close()
    print(f"{NAMES[h]}: registry.k8s.io hosts.toml OK")
    c.close()
print("registry.k8s.io mirror patched")
