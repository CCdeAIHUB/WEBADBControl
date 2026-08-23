#!/usr/bin/env sh
set -eu

if [ "$(id -u)" -ne 0 ]; then
  echo "请使用 root 权限运行安装脚本。" >&2
  exit 1
fi

if command -v apt-get >/dev/null 2>&1; then
  apt-get update
  apt-get install -y adb ca-certificates curl unzip
elif command -v dnf >/dev/null 2>&1; then
  dnf install -y android-tools ca-certificates curl unzip
elif command -v pacman >/dev/null 2>&1; then
  pacman -Sy --needed --noconfirm android-tools ca-certificates curl unzip
elif command -v zypper >/dev/null 2>&1; then
  zypper --non-interactive install android-tools ca-certificates curl unzip
else
  echo "未识别包管理器，请手动安装 Android platform-tools。" >&2
  exit 1
fi

id webadb >/dev/null 2>&1 || useradd --system --home-dir /opt/webadbcontrol --shell /usr/sbin/nologin webadb
install -d -o webadb -g webadb /opt/webadbcontrol /var/lib/webadbcontrol /etc/webadbcontrol
install -m 0755 webadbcontrol adbcontrol-core /opt/webadbcontrol/
install -m 0755 core/scripts/adb/verify-adb-capabilities.sh /opt/webadbcontrol/verify-adb-capabilities
case "$(uname -m)" in
  x86_64|amd64)
    adb_target="linux-x86_64"
    adb_work_dir="$(mktemp -d)"
    trap 'rm -rf "${adb_work_dir}"' EXIT
    curl --fail --location --retry 3 --output "${adb_work_dir}/platform-tools.zip" "https://dl.google.com/android/repository/platform-tools-latest-linux.zip"
    unzip -q "${adb_work_dir}/platform-tools.zip" -d "${adb_work_dir}"
    install -d "/opt/webadbcontrol/assets/adb/${adb_target}"
    install -m 0755 "${adb_work_dir}/platform-tools/adb" "/opt/webadbcontrol/assets/adb/${adb_target}/adb"
    ;;
  aarch64|arm64)
    adb_target="linux-arm64"
    install -d "/opt/webadbcontrol/assets/adb/${adb_target}"
    install -m 0755 "$(command -v adb)" "/opt/webadbcontrol/assets/adb/${adb_target}/adb"
    ;;
  *)
    echo "当前 CPU 架构没有声明 ADB 资产：$(uname -m)" >&2
    exit 1
    ;;
esac
/opt/webadbcontrol/verify-adb-capabilities "/opt/webadbcontrol/assets/adb/${adb_target}/adb"
cp -R web /opt/webadbcontrol/web
install -m 0644 deploy/systemd/webadbcontrol.service /etc/systemd/system/webadbcontrol.service
if [ ! -f /etc/webadbcontrol/webadbcontrol.env ]; then
  install -m 0600 deploy/systemd/webadbcontrol.env.example /etc/webadbcontrol/webadbcontrol.env
fi
systemctl daemon-reload
systemctl enable webadbcontrol.service
echo "安装完成。请先编辑 /etc/webadbcontrol/webadbcontrol.env，再运行 systemctl start webadbcontrol。"
