#!/usr/bin/env sh
set -eu

adb_binary="${1:-}"
if [ -z "${adb_binary}" ] || [ ! -x "${adb_binary}" ]; then
  echo "ADB binary is missing or not executable: ${adb_binary}" >&2
  exit 1
fi

help_output="$(${adb_binary} help 2>&1)"
for capability in pair mdns; do
  if ! printf '%s\n' "${help_output}" | grep -Eq "(^|[[:space:]])${capability}([[:space:]]|$)"; then
    echo "ADB binary does not support required command: ${capability}" >&2
    "${adb_binary}" version >&2 || true
    exit 1
  fi
done

"${adb_binary}" version
