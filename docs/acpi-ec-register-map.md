# ACPI EC Register Map

This document records the currently validated EC register layout for IP3-style fan RPM telemetry.

## EC operation region

Validated fan-related EC offsets:

| Offset | ACPI Field | Meaning | Access in this project |
|---:|---|---|---|
| 0x31 | FCMO | Current power/fan mode | read-only diagnostic |
| 0x32 | FCMI | Mode input/control | never written |
| 0x33 | FAN1 | Fan 1 control byte | never written |
| 0x34 | FAN2 | Fan 2 control byte | never written |
| 0x35 | FN1L | Fan 1 RPM high byte by ACPI formula | read-only |
| 0x36 | FN1H | Fan 1 RPM low byte by ACPI formula | read-only |
| 0x37 | FN2L | Fan 2 RPM high byte by ACPI formula | read-only |
| 0x38 | FN2H | Fan 2 RPM low byte by ACPI formula | read-only |

## RPM formula

The firmware ACPI method combines the bytes as:

    fan1_rpm = EC[0x36] + (EC[0x35] << 8)
    fan2_rpm = EC[0x38] + (EC[0x37] << 8)

The byte order follows the observed ACPI method, not the field names alone.

## Safety boundary

The driver must not write:

- EC[0x32]
- EC[0x33]
- EC[0x34]
- any other EC register

The first hwmon driver must be read-only.
