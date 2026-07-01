# Kernel hwmon Driver Plan

## Goal

Expose IP3-style EC fan RPM as standard Linux hwmon attributes:

    /sys/class/hwmon/hwmonX/name
    /sys/class/hwmon/hwmonX/fan1_input
    /sys/class/hwmon/hwmonX/fan1_label
    /sys/class/hwmon/hwmonX/fan2_input
    /sys/class/hwmon/hwmonX/fan2_label

## Non-goals

- no fan control
- no PWM
- no EC writes
- no power profile switching
- no vendor WMI method calls in v1

## Proposed driver name

    ip3_ec_hwmon

## Proposed module parameters

    force=0
    enable_fan2=0

## Matching strategy

Default auto-bind:

1. DMI allowlist match
2. IP3 ACPI/WMI signals present
3. EC read succeeds
4. fan1 value is plausible

Experimental bind:

    modprobe ip3_ec_hwmon force=1

## Read path

Use kernel EC read interface, not debugfs.

The userspace prototype reads:

    /sys/kernel/debug/ec/ec0/io

The kernel driver should use the ACPI EC access API where available.

## hwmon ops

The driver should implement:

- hwmon_fan_input
- hwmon_fan_label

fan1_input:

    EC[0x36] + (EC[0x35] << 8)

fan2_input:

    EC[0x38] + (EC[0x37] << 8)

## Safety

The driver must not write to EC.

Writable hwmon attributes must not be registered.
