from __future__ import annotations

import argparse
import json
import time
from pathlib import Path
from typing import Any


EC_IO = Path("/sys/kernel/debug/ec/ec0/io")
DMI_DIR = Path("/sys/class/dmi/id")
ACPI_DEVICES = Path("/sys/bus/acpi/devices")
WMI_DEVICES = Path("/sys/bus/wmi/devices")


def read_text(path: Path) -> str | None:
    try:
        return path.read_text(errors="replace").strip()
    except Exception:
        return None


def read_dmi_field(name: str) -> str | None:
    return read_text(DMI_DIR / name)


def has_acpi_uid(value: str) -> bool:
    if not ACPI_DEVICES.exists():
        return False

    for uid_file in ACPI_DEVICES.glob("PNP0C14:*/uid"):
        if read_text(uid_file) == value:
            return True
    return False


def has_wmi_guid(guid: str) -> bool:
    if not WMI_DEVICES.exists():
        return False

    target = guid.upper()
    for item in WMI_DEVICES.iterdir():
        if item.name.upper() == target:
            return True
    return False


def standard_hwmon_fan_available() -> bool:
    hwmon = Path("/sys/class/hwmon")
    if not hwmon.exists():
        return False

    for path in hwmon.glob("hwmon*/fan*_input"):
        if path.is_file():
            return True
    return False


def read_ec_fans(ec_io: Path = EC_IO) -> dict[str, Any]:
    if not ec_io.exists():
        return {
            "rpm_available": False,
            "error": "EC IO path not found. Try loading ec_sys and mounting debugfs.",
            "ec_io_path": str(ec_io),
        }

    try:
        data = ec_io.read_bytes()
    except PermissionError:
        return {
            "rpm_available": False,
            "error": "Permission denied reading EC IO. Run as root or configure read-only access carefully.",
            "ec_io_path": str(ec_io),
        }
    except Exception as exc:
        return {
            "rpm_available": False,
            "error": f"Failed to read EC IO: {exc}",
            "ec_io_path": str(ec_io),
        }

    if len(data) < 0x39:
        return {
            "rpm_available": False,
            "error": f"EC IO too short: {len(data)} bytes",
            "ec_io_path": str(ec_io),
        }

    fcmo = data[0x31]
    fcmi = data[0x32]
    fan1_control = data[0x33]
    fan2_control = data[0x34]

    fn1l = data[0x35]
    fn1h = data[0x36]
    fn2l = data[0x37]
    fn2h = data[0x38]

    # Based on the IP3 WMAA ACPI method:
    # Local2 = FN1H + (FN1L << 8) + (FN2H << 16) + (FN2L << 24)
    fan1_rpm = fn1h + (fn1l << 8)
    fan2_rpm = fn2h + (fn2l << 8)

    return {
        "rpm_available": True,
        "ec_io_path": str(ec_io),
        "source": "acpi_ec",
        "fan1_rpm": fan1_rpm,
        "fan2_rpm": fan2_rpm,
        "fan1_available": fan1_rpm > 0,
        "fan2_available": fan2_rpm > 0,
        "mode": {
            "fcmo_0x31": fcmo,
            "fcmi_0x32": fcmi,
        },
        "control_bytes_read_only": {
            "fan1_0x33": fan1_control,
            "fan2_0x34": fan2_control,
        },
        "raw": {
            "ec_0x35_fn1l": fn1l,
            "ec_0x36_fn1h": fn1h,
            "ec_0x37_fn2l": fn2l,
            "ec_0x38_fn2h": fn2h,
            "ec_0x31_0x38_hex": data[0x31:0x39].hex(" "),
        },
        "formula": {
            "fan1": "EC[0x36] + (EC[0x35] << 8)",
            "fan2": "EC[0x38] + (EC[0x37] << 8)",
        },
    }


def snapshot() -> dict[str, Any]:
    return {
        "tool": "ip3-ec-fan-rpm",
        "version": "0.1.0",
        "mode": "read_only",
        "safety": {
            "writes_ec": False,
            "requires_ec_write_support": False,
            "controls_fan": False,
            "changes_power_profile": False,
        },
        "dmi": {
            "sys_vendor": read_dmi_field("sys_vendor"),
            "product_name": read_dmi_field("product_name"),
            "product_version": read_dmi_field("product_version"),
            "board_vendor": read_dmi_field("board_vendor"),
            "board_name": read_dmi_field("board_name"),
            "bios_vendor": read_dmi_field("bios_vendor"),
            "bios_version": read_dmi_field("bios_version"),
            "bios_date": read_dmi_field("bios_date"),
        },
        "signals": {
            "ip3_power_switch": has_acpi_uid("IP3POWERSWITCH"),
            "ip3_wmi_event": has_acpi_uid("IP3WMIEVENT"),
            "rwec_reg_wmi": has_acpi_uid("RWECREGWMI"),
            "wmi_guid_99d89064": has_wmi_guid("99D89064-8D50-42BB-BEA9-155B2E5D0FCD"),
            "wmi_guid_8fafc061": has_wmi_guid("8FAFC061-22DA-46E2-91DB-1FE3D7E5FF3C"),
            "standard_hwmon_fan_available": standard_hwmon_fan_available(),
            "ec_io_exists": EC_IO.exists(),
        },
        "fan": read_ec_fans(),
    }


def to_prometheus(s: dict[str, Any]) -> str:
    fan = s["fan"]
    signals = s["signals"]
    dmi = s["dmi"]

    labels = {
        "vendor": dmi.get("sys_vendor") or "unknown",
        "product": dmi.get("product_name") or "unknown",
    }

    def label_text(extra: dict[str, str] | None = None) -> str:
        merged = dict(labels)
        if extra:
            merged.update(extra)
        return ",".join(f'{k}="{v}"' for k, v in merged.items())

    lines: list[str] = []

    lines.append("# HELP ip3_ec_readable Whether EC fan registers are readable")
    lines.append("# TYPE ip3_ec_readable gauge")
    lines.append(f"ip3_ec_readable{{{label_text()}}} {1 if fan.get('rpm_available') else 0}")

    lines.append("# HELP ip3_ec_signal_present Whether expected IP3 ACPI/WMI signals are present")
    lines.append("# TYPE ip3_ec_signal_present gauge")
    for name in [
        "ip3_power_switch",
        "ip3_wmi_event",
        "rwec_reg_wmi",
        "wmi_guid_99d89064",
        "wmi_guid_8fafc061",
    ]:
        value = 1 if signals.get(name) else 0
        lines.append(f'ip3_ec_signal_present{{{label_text({"signal": name})}}} {value}')

    lines.append("# HELP ip3_ec_standard_hwmon_fan_available Whether fan RPM is exposed through standard Linux hwmon")
    lines.append("# TYPE ip3_ec_standard_hwmon_fan_available gauge")
    lines.append(f"ip3_ec_standard_hwmon_fan_available{{{label_text()}}} {1 if signals.get('standard_hwmon_fan_available') else 0}")

    lines.append("# HELP ip3_ec_fan_rpm Fan RPM read from ACPI EC")
    lines.append("# TYPE ip3_ec_fan_rpm gauge")
    if fan.get("rpm_available"):
        lines.append(f'ip3_ec_fan_rpm{{{label_text({"fan": "1", "source": "acpi_ec"})}}} {fan["fan1_rpm"]}')
        lines.append(f'ip3_ec_fan_rpm{{{label_text({"fan": "2", "source": "acpi_ec"})}}} {fan["fan2_rpm"]}')

    return "\n".join(lines) + "\n"


def print_human(s: dict[str, Any]) -> None:
    dmi = s["dmi"]
    signals = s["signals"]
    fan = s["fan"]

    print(f"{dmi.get('sys_vendor') or 'unknown'} {dmi.get('product_name') or 'unknown'}")
    print(f"BIOS: {dmi.get('bios_version') or 'unknown'} ({dmi.get('bios_date') or 'unknown'})")
    print(f"mode: {s['mode']}")

    print()
    print("signals:")
    for key, value in signals.items():
        print(f"  {key}: {value}")

    print()
    if fan.get("rpm_available"):
        print(f"fan1: {fan['fan1_rpm']} rpm")
        print(f"fan2: {fan['fan2_rpm']} rpm")
        print(f"raw:  {fan['raw']['ec_0x31_0x38_hex']}")
        print("source: ACPI EC, read-only")
    else:
        print(f"fan rpm unavailable: {fan.get('error')}")


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Read-only fan RPM reader for IP3-style AMD mini PCs using ACPI EC registers."
    )
    parser.add_argument("--json", action="store_true", help="Print JSON output")
    parser.add_argument("--prometheus", action="store_true", help="Print Prometheus text output")
    parser.add_argument("--watch", type=float, default=0, help="Repeat every N seconds")
    args = parser.parse_args()

    while True:
        s = snapshot()

        if args.prometheus:
            print(to_prometheus(s), end="")
        elif args.json:
            print(json.dumps(s, ensure_ascii=False, indent=2))
        else:
            print_human(s)

        if not args.watch:
            break
        try:
            time.sleep(args.watch)
        except KeyboardInterrupt:
            print()
            break


if __name__ == "__main__":
    main()
