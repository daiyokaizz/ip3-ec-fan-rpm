# hwmon Driver Contract

The future kernel driver should expose read-only fan RPM through the Linux hwmon subsystem.

## Driver scope

The first driver version must be read-only.

It may expose:

- name
- fan1_input
- fan1_label
- fan2_input, if valid
- fan2_label, if valid

It must not expose:

- pwm*
- fan*_target
- writable fan controls
- power profile controls
- EC write paths

## Device matching

The driver should only bind automatically when the device is known safe.

Recommended matching gates:

1. DMI allowlist match
2. ACPI/WMI IP3-style signals present:
   - IP3POWERSWITCH
   - RWECREGWMI
   - WMI GUID 99D89064-8D50-42BB-BEA9-155B2E5D0FCD
3. EC read path available from kernel EC interface
4. fan1 RPM is plausible

Experimental loading may be allowed with a module parameter:

    force=1

When force=1 is used, the driver should log a warning.

## hwmon mapping

fan1_input:

    EC[0x36] + (EC[0x35] << 8)

fan2_input:

    EC[0x38] + (EC[0x37] << 8)

fan2 may be hidden or reported as unavailable when it is consistently zero.

## Plausibility checks

A valid RPM should usually be:

    0 <= rpm <= 10000

0 may mean:

- fan stopped
- fan tach unavailable
- unused fan channel

The driver should not fail only because fan2 is 0.

## Failure behavior

If the device does not match:

- do not register hwmon device
- return -ENODEV

If EC read fails:

- return an error from fan read callback
- do not attempt any write recovery

## Safety rules

The driver must never write EC registers in the first version.
