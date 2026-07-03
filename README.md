# ip3-ec-fan-rpm

Read-only fan RPM reader for IP3-style AMD mini PCs using ACPI EC registers.

Some Linux systems expose CPU, GPU, NVMe, and network device temperatures through standard sensors, but do not expose fan RPM through standard hwmon paths such as:

    /sys/class/hwmon/hwmon*/fan*_input

On some IP3-style AMD mini PCs, the fan tachometer value is available through ACPI Embedded Controller registers instead. This tool reads those EC bytes and reports fan RPM without changing fan speed, power mode, or firmware state.

## Status

Early alpha.

Confirmed on one IP3-style AMD mini PC with an ACPI/WMI tree exposing:

    IP3WMIEVENT
    IP3POWERSWITCH
    RWECREGWMI

Additional systems need validation.

## Safety

This tool is read-only.

It does not:

- enable ec_sys write_support
- write to /sys/kernel/debug/ec/ec0/io
- change fan speed
- change fan PWM
- change power profile
- call WMI methods
- load out-of-tree kernel modules
- run pwmconfig
- run fancontrol
- write arbitrary EC registers

It only reads EC bytes.

## License and commercial use

This project is source-available, not OSI-open-source.

Personal, educational, research, hobbyist, and other non-commercial use is allowed.

Commercial use requires prior written permission from the copyright holder. This includes use by hardware vendors, OEMs, ODMs, cloud providers, system integrators, repair providers, managed service providers, enterprise IT operations, and commercial monitoring or support products.

See `LICENSE` for details.

## Requirements

Linux with:

- ec_sys kernel module
- mounted debugfs
- permission to read /sys/kernel/debug/ec/ec0/io

Typical setup:

    sudo modprobe ec_sys
    sudo mount -t debugfs none /sys/kernel/debug 2>/dev/null || true

Then run the tool as root or with equivalent read permission.

## EC register layout

On the validated system, the firmware DSDT defines fan RPM fields in the EC operation region:

    EC[0x35] = FN1L
    EC[0x36] = FN1H
    EC[0x37] = FN2L
    EC[0x38] = FN2H

The ACPI method combines them as:

    fan1_rpm = EC[0x36] + (EC[0x35] << 8)
    fan2_rpm = EC[0x38] + (EC[0x37] << 8)

This byte order follows the observed ACPI method, not the field names alone.

## Install for local development

    python3 -m venv .venv
    . .venv/bin/activate
    pip install -e .

## Go single-binary MVP quick start

Build the Go MVP to a temporary path so the repository stays free of local binaries:

    go build -o /tmp/ip3-ec-fan-rpm-go ./cmd/ip3-ec-fan-rpm

Run it with read-only EC access:

    sudo /tmp/ip3-ec-fan-rpm-go
    sudo /tmp/ip3-ec-fan-rpm-go --json
    sudo /tmp/ip3-ec-fan-rpm-go --prometheus

For node_exporter textfile collection, pass an explicit collector path:

    sudo /tmp/ip3-ec-fan-rpm-go --textfile /var/lib/node_exporter/textfile_collector/ip3_ec_fan.prom

## Usage

Human-readable output:

    sudo ip3-ec-fan-rpm

JSON output:

    sudo ip3-ec-fan-rpm --json

Prometheus text output:

    sudo ip3-ec-fan-rpm --prometheus

Watch mode:

    sudo ip3-ec-fan-rpm --watch 1

## Example output

Human-readable output may look like:

    vendor product
    BIOS: unknown
    mode: read_only

    signals:
      ip3_power_switch: true
      ip3_wmi_event: true
      rwec_reg_wmi: true
      wmi_guid_99d89064: true
      wmi_guid_8fafc061: true
      standard_hwmon_fan_available: false
      ec_io_exists: true

    fan1: 1620 rpm
    fan2: 0 rpm
    source: ACPI EC, read-only

The second fan may report 0 if the system has only one EC-connected fan tachometer input.

## Prometheus metrics

Example Prometheus text output:

    ip3_ec_readable{vendor="unknown",product="unknown"} 1
    ip3_ec_signal_present{vendor="unknown",product="unknown",signal="ip3_power_switch"} 1
    ip3_ec_standard_hwmon_fan_available{vendor="unknown",product="unknown"} 0
    ip3_ec_fan_rpm{vendor="unknown",product="unknown",fan="1",source="acpi_ec"} 1620
    ip3_ec_fan_rpm{vendor="unknown",product="unknown",fan="2",source="acpi_ec"} 0

## What this project is

This project is a small read-only observability tool.

It is useful when:

- the physical fan exists
- lm-sensors works for temperatures but shows no fan RPM
- /sys/class/hwmon has no fan*_input
- the system exposes IP3-style ACPI/WMI entries
- the EC fan RPM bytes are populated by firmware

## What this project is not

This project is not a fan controller.

It does not attempt to:

- control fan PWM
- override fan curves
- switch Quiet / Balanced / Performance modes
- write EC registers
- replace vendor firmware logic
- provide universal support for all mini PCs

## Validation checklist

Before claiming support for a new system, collect:

    sudo modprobe ec_sys
    sudo mount -t debugfs none /sys/kernel/debug 2>/dev/null || true
    sudo ip3-ec-fan-rpm --json
    sudo ip3-ec-fan-rpm --watch 1

Recommended checks:

- fan1_rpm should be in a plausible range, for example 800 to 7000 RPM
- RPM should vary slightly over time
- RPM should rise under sustained load
- RPM should fall again after load stops
- fan2_rpm may be 0 on single-fan systems

## Privacy

Please avoid posting private hostnames, serial numbers, local IP addresses, account names, or full DMI dumps in public issues.

When reporting hardware compatibility, prefer a minimal summary:

    CPU family:
    GPU family:
    Kernel version:
    BIOS date or version, if comfortable sharing:
    IP3 signals present:
    fan1 RPM observed:
    fan2 RPM observed:

Do not include serial numbers.

## Known limitations

- Requires root or equivalent permission to read EC debugfs.
- Requires ec_sys and debugfs.
- Does not create a standard hwmon fan*_input device.
- Does not support fan control.
- EC register layout may differ across vendors or BIOS versions.
- A similar ACPI/WMI tree does not guarantee that fan RPM registers are populated.

## Related concepts

- ACPI Embedded Controller
- Linux ec_sys
- Linux debugfs
- Linux hwmon
- IP3-style ACPI/WMI firmware
- Prometheus textfile collector

## License

This project is source-available for personal, educational, research, hobbyist, and other non-commercial use.

Commercial use requires prior written permission from the copyright holder.

See `LICENSE` for details.
