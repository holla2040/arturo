# Arturo Protocol Schemas

JSON Schema definitions for the Arturo messaging protocol. These are the single source of truth for all messages exchanged between the Go server and ESP32 field devices over Redis.

Schema versions use directory-based versioning:

```
schemas/
└── v1.0.0/                        # Protocol version
    ├── README.md                   # Schema index and overview
    ├── envelope/                   # Shared message envelope
    │   └── schema-definition.md
    ├── error/                      # Shared error object
    │   └── schema-definition.md
    ├── device-command-request/     # Server -> ESP32 command
    │   ├── schema-definition.md
    │   └── examples/
    ├── device-command-response/    # ESP32 -> Server result
    │   ├── schema-definition.md
    │   └── examples/
    ├── service-heartbeat/          # ESP32 health report
    │   ├── schema-definition.md
    │   └── examples/
    ├── system-emergency-stop/      # E-stop broadcast
    │   ├── schema-definition.md
    │   └── examples/
    └── system-ota-request/         # Firmware update request
        ├── schema-definition.md
        └── examples/
```

Each `schema-definition.md` contains the complete JSON Schema, field descriptions, usage examples, and implementation details for both Go server and ESP32 firmware.
