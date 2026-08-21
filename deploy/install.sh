#!/usr/bin/env sh
set -eu

if [ "$(id -u)" -ne 0 ]; then
  echo "请使用 root 权限运行安装脚本。" >&2
  exit 1
fi

if command -v apt-get >/dev/null 2>&1; then
  apt-get update
  apt-get install -y adb ca-certificates
elif command -v dnf >/dev/null 2>&1; then
  dnf install -y android-tools ca-certificates
elif command -v pacman >/dev/null 2>&1; then
  pacman -Sy --needed --noconfirm android-tools ca-certificates
elif command -v zypper >/dev/null 2>&1; then
  zypper --non-interactive install android-tools ca-certificates
else
  echo "未识别包管理器，请手动安装 Android platform-tools。" >&2
  exit 1
fi

id webadb >/dev/null 2>&1 || useradd --system --home-dir /opt/webadbcontrol --shell /usr/sbin/nologin webadb
install -d -o webadb -g webadb /opt/webadbcontrol /var/lib/webadbcontrol /etc/webadbcontrol
install -m 0755 webadbcontrol adbcontrol-core /opt/webadbcontrol/
cp -R web /opt/webadbcontrol/web
install -m 0644 deploy/systemd/webadbcontrol.service /etc/systemd/system/webadbcontrol.service
if [ ! -f /etc/webadbcontrol/webadbcontrol.env ]; then
  install -m 0600 deploy/systemd/webadbcontrol.env.example /etc/webadbcontrol/webadbcontrol.env
fi
systemctl daemon-reload
systemctl enable webadbcontrol.service
echo "安装完成。请先编辑 /etc/webadbcontrol/webadbcontrol.env，再运行 systemctl start webadbcontrol。"
