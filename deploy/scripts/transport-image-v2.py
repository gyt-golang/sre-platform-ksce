#!/usr/bin/env python3
# node02 export chaos-daemon 镜像 -> 内网 scp 到 master01/node01/master03 -> ctr import
import os, paramiko
PWD = os.environ["KSCE_PWD"]
NODE02 = "10.0.0.242"
TARGETS = [("10.0.0.182","master01","10.0.0.182"),("10.0.0.136","node01","10.0.0.136"),("10.0.0.107","master03","10.0.0.107")]
IMG = "ghcr.io/chaos-mesh/chaos-daemon:v2.7.0"
TAR = "/tmp/chaos-daemon.tar"

def conn(h):
    c=paramiko.SSHClient(); c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    c.connect(h,username="root",password=PWD,timeout=15,allow_agent=False,look_for_keys=False)
    return c
def run(c,cmd,t=300):
    _,o,e=c.exec_command(cmd,timeout=t); rc=o.channel.recv_exit_status()
    return rc,(o.read().decode("utf-8","replace")+e.read().decode("utf-8","replace")).strip()

# 1. node02: ensure ssh key + get pubkey
c=conn(NODE02)
run(c,"[ -f /root/.ssh/id_rsa ] || ssh-keygen -t rsa -N '' -q -f /root/.ssh/id_rsa")
rc,pub=run(c,"cat /root/.ssh/id_rsa.pub")
print(f"[node02] pubkey ready: {pub[:40]}...")

# 2. append pubkey to 3 targets authorized_keys
for ip,name,ext in TARGETS:
    t=conn(ext)
    run(t,f"mkdir -p /root/.ssh; grep -qxF '{pub}' /root/.ssh/authorized_keys 2>/dev/null || echo '{pub}' >> /root/.ssh/authorized_keys; chmod 700 /root/.ssh; chmod 600 /root/.ssh/authorized_keys")
    t.close()
print("[targets] authorized_keys updated")

# 3. node02: export tar
rc,out=run(c,f"ctr -n k8s.io images export --platform linux/amd64 {TAR} {IMG} 2>&1; ls -lh {TAR}")
print(f"[node02] export -> {out.replace(chr(10),' | ')[:150]}")

# 4. node02: scp to 3 nodes via internal IP
for ip,name,ext in TARGETS:
    rc,out=run(c,f"scp -o StrictHostKeyChecking=no -o BatchMode=yes -o ConnectTimeout=10 {TAR} root@{ip}:{TAR} 2>&1",t=120)
    print(f"[node02-> {name} {ip}] scp {'OK' if rc==0 else 'FAIL'}: {out[:80]}")
c.close()

# 5. each target: ctr import
for ip,name,ext in TARGETS:
    t=conn(ext)
    rc,out=run(t,f"ctr -n k8s.io images import --no-unpack {TAR} 2>&1 | tail -2; crictl images 2>/dev/null | grep chaos-daemon | head -1")
    print(f"[{name}] import -> {out.replace(chr(10),' | ')[:120]}")
    t.close()
print("done")
