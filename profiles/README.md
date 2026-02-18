# Device Profiles

YAML definitions for each instrument type. These describe the protocol, connection parameters, and command vocabulary for each device.

Profiles are compiled into ESP32 firmware or used as reference by the server's device registry.

## Structure

```
profiles/
├── testequipment/     # DMMs, oscilloscopes, power supplies (SCPI)
├── controllers/       # Arduino, temperature controllers
├── pumps/             # Cryopumps (CTI protocol)
├── relays/            # Relay boards (GPIO/USB)
└── modbus/            # Modbus RTU/TCP devices
```

## Carried Forward

These profiles were validated in the original arturo-go-archive project.
