#!/usr/bin/env bash
set -euo pipefail

echo "== uname =="
uname -a

echo
echo "== DMI =="
for f in sys_vendor product_name product_version board_vendor board_name bios_vendor bios_version bios_date; do
  p="/sys/class/dmi/id/$f"
  if [ -r "$p" ]; then
    echo "$f: $(cat "$p")"
  fi
done

echo
echo "== ACPI PNP0C14 UIDs =="
for p in /sys/bus/acpi/devices/PNP0C14:*/uid; do
  [ -r "$p" ] || continue
  echo "$p: $(cat "$p")"
done

echo
echo "== WMI devices =="
if [ -d /sys/bus/wmi/devices ]; then
  find /sys/bus/wmi/devices -maxdepth 1 -mindepth 1 -printf '%f\n' | sort
fi

echo
echo "== Standard hwmon fan inputs =="
find /sys/class/hwmon -type f -name 'fan*_input' -print 2>/dev/null || true

echo
echo "== EC debugfs read check =="
if [ "$(id -u)" -ne 0 ]; then
  echo "Not root. Re-run with sudo to collect EC bytes."
  exit 0
fi

modprobe ec_sys || true
mountpoint -q /sys/kernel/debug || mount -t debugfs none /sys/kernel/debug

EC="/sys/kernel/debug/ec/ec0/io"
if [ ! -r "$EC" ]; then
  echo "EC io not readable: $EC"
  exit 0
fi

echo "EC path: $EC"
echo "EC bytes 0x30-0x3f:"
dd if="$EC" bs=1 count=16 skip=$((0x30)) 2>/dev/null | xxd -g1

echo
echo "Decoded fan RPM:"
python3 - <<'PY'
from pathlib import Path

p = Path("/sys/kernel/debug/ec/ec0/io")
data = p.read_bytes()

fan1 = data[0x36] + (data[0x35] << 8)
fan2 = data[0x38] + (data[0x37] << 8)

print(f"fan1_rpm={fan1}")
print(f"fan2_rpm={fan2}")
print(f"raw_0x31_0x38={data[0x31:0x39].hex(' ')}")
PY
