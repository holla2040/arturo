# Arturo Messaging Protocol v2.0 - Specification

**STATUS: ACTIVE**
**VERSION: 2.0**

## 1. Overview and Core Principles

This document is the **Single Source of Truth (SSoT)** for the Arturo Messaging Protocol. It defines the non-negotiable rules and contracts for all inter-service communication within the Arturo ecosystem. All components, without exception, MUST adhere to this specification.

The protocol is built on three core principles:

1.  **Contract-First with JSON Schema:** All message payloads are defined by a formal [JSON Schema](https://json-schema.org/). The schema is the contract; the documentation is for human reference. This eliminates ambiguity and prevents schema drift through automated validation.
2.  **Asynchronous Communication via Redis Pub/Sub:** All messages are exchanged asynchronously over Redis channels. This decouples services and enhances system resilience.
3.  **Single Source of Truth for Functionality:** As a foundational architectural rule, any given capability (e.g., script parsing, device control) is owned by exactly ONE service. Services MUST communicate via this protocol to access capabilities owned by others.

**Deviation from this specification is a bug.**

---

## 2. The Contract: JSON Schema

All message payload structures are defined in JSON Schema files located in the `/schemas` directory of the project repository.

-   **Location:** `/schemas/{message_type}/{version}.json` (e.g., `/schemas/device.discovered/v1.json`)
-   **Authority:** The schema file is the ground truth, not this document's examples.
-   **Validation:** Every service MUST validate all incoming messages against the corresponding JSON schema. Messages that fail validation MUST be rejected and logged as a `E_VALIDATION_FAILED` error.

## 3. Message Envelope Structure

All messages published to Redis MUST use the following envelope structure.

```json
{
  "envelope": {
    "id": "msg-a1b2c3d4-e5f6-7890-1234-567890abcdef",
    "timestamp": "2025-07-18T10:00:00.12345Z",
    "source": {
      "service": "service-name",
      "instance": "instance-id",
      "version": "1.2.0"
    },
    "schema_version": "v1",
    "type": "message.type.verb",
    "correlation_id": "corr-a1b2-c3d4-e5f6",
    "trace_id": "trace-a1b2-c3d4-e5f6",
    "reply_to": "responses/service-name/instance-id",
    "auth": {
      "jwt": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
    }
  },
  "payload": {
    // Schema-validated payload goes here
  }
}
```

### Envelope Fields:

| Field | Type | Description | Required? |
| :--- | :--- | :--- | :--- |
| **id** | `string` | UUIDv4. A unique identifier for this specific message. | **Yes** |
| **timestamp** | `string` | UTC timestamp in RFC3339 format, including milliseconds. | **Yes** |
| **source** | `object` | Contains information about the message originator. | **Yes** |
| ├ `service` | `string` | The official name of the source service (e.g., `tcp_manager_service`). | **Yes** |
| ├ `instance`| `string` | A unique identifier for the running instance of the service. | **Yes** |
| ├ `version` | `string` | The semantic version of the source service. | **Yes** |
| **schema_version** | `string` | The version of the payload schema being used (e.g., "v1", "v2"). | **Yes** |
| **type** | `string` | The message type, indicating its intent (e.g., `device.command.request`). | **Yes** |
| **correlation_id** | `string` | ID used to link messages within a single workflow (e.g., a request and its response). | No |
| **trace_id** | `string` | ID used for distributed tracing across multiple workflows. | No |
| **reply_to** | `string` | The specific channel a response should be sent to. **Required for request/response patterns.** | No |
| **auth** | `object` | Contains the authentication token for the request. | No |
| ├ `jwt` | `string` | A JSON Web Token issued by the `auth_service`. | No |
| **payload** | `object` | The actual message content, which MUST conform to its JSON Schema. | **Yes** |

---

## 4. Channel Naming Conventions

Channels remain hierarchical. This structure is mandatory.

| Category | Pattern | Description |
| :--- | :--- | :--- |
| **Commands** | `commands/{service-name}` | For sending commands to a specific service type. |
| **Responses** | `responses/{service-name}/{instance-id}` | For sending responses back to a specific service instance. |
| **Events** | `events/{event-source}/{event-name}` | For broadcasting domain events (e.g., `events/device/discovered`). |
| **System** | `system/{event-name}` | For system-wide broadcasts (e.g., `system/emergency_stop`, `system/heartbeat`). |

---

## 5. Core Message Patterns

### Pattern 1: Request/Response (e.g., Executing a Command)

This is the pattern for all interactions where a service needs another service to perform work and return a result.

**Step 1: The Request**
Service `script_executor` (instance `se-01`) sends a command to the `tcp_manager_service`.

- **Channel:** `commands/tcp_manager_service`
- **Message:**
```json
{
  "envelope": {
    "id": "msg-1111-2222-3333-4444",
    "timestamp": "2025-07-18T11:00:00Z",
    "source": { "service": "script_executor", "instance": "se-01", "version": "1.0.0" },
    "schema_version": "v1",
    "type": "device.command.request",
    "correlation_id": "corr-abcd",
    "reply_to": "responses/script_executor/se-01"
  },
  "payload": {
    "device_id": "dmm-01",
    "command_name": "MEAS:VOLT:DC?",
    "parameters": ["10", "MAX"],
    "timeout_ms": 5000
  }
}
```

**Step 2: The Response (Success)**
The `tcp_manager_service` successfully executes the command and sends a response.

- **Channel:** `responses/script_executor/se-01` (from the `reply_to` field)
- **Message:**
```json
{
  "envelope": {
    "id": "msg-5555-6666-7777-8888",
    "timestamp": "2025-07-18T11:00:01Z",
    "source": { "service": "tcp_manager_service", "instance": "tm-01", "version": "1.2.0" },
    "schema_version": "v1",
    "type": "device.command.response",
    "correlation_id": "corr-abcd"
  },
  "payload": {
    "device_id": "dmm-01",
    "command_name": "MEAS:VOLT:DC?",
    "response": "5.0012",
    "error": null,
    "duration_ms": 125
  }
}
```

**Step 3: The Response (Failure)**
Alternatively, the `tcp_manager_service` fails to execute the command.

- **Channel:** `responses/script_executor/se-01`
- **Message:**
```json
{
  "envelope": {
    "id": "msg-9999-aaaa-bbbb-cccc",
    "timestamp": "2025-07-18T11:00:05Z",
    "source": { "service": "tcp_manager_service", "instance": "tm-01", "version": "1.2.0" },
    "schema_version": "v1",
    "type": "device.command.response",
    "correlation_id": "corr-abcd"
  },
  "payload": {
    "device_id": "dmm-01",
    "command_name": "MEAS:VOLT:DC?",
    "response": null,
    "error": {
      "code": "E_DEVICE_TIMEOUT",
      "message": "Timeout waiting for response from device dmm-01.",
      "details": {
        "timeout_ms": 5000,
        "address": "192.168.1.50:5025"
      }
    },
    "duration_ms": 5001
  }
}
```

### Pattern 2: Service Heartbeat (Health Monitoring)

Services send periodic heartbeat messages to indicate they are healthy and operational.

**The Heartbeat Message**
Each service publishes a heartbeat to the system channel every 30 seconds.

- **Channel:** `system/heartbeat`
- **Message Type:** `service.heartbeat`
- **Message:**
```json
{
  "envelope": {
    "id": "msg-1234-5678-90ab-cdef",
    "timestamp": "2025-07-18T11:00:00.123Z",
    "source": { "service": "tcp_manager_service", "instance": "tm-01", "version": "1.0.0" },
    "schema_version": "v1",
    "type": "service.heartbeat"
  },
  "payload": {
    "status": "healthy",
    "uptime_seconds": 3600,
    "connected_devices": 3,
    "active_devices": 2,
    "metrics": {
      "message_count": 1500,
      "error_count": 5,
      "memory_mb": 128
    }
  }
}
```

**Monitoring Services:**
- Services that monitor health (e.g., `terminal_dashboard`) subscribe to `system/heartbeat`
- Services are considered unhealthy if no heartbeat is received within 2 minutes
- The heartbeat payload is defined in `/schemas/service.heartbeat/v1.json`

### Pattern 4: Service Lifecycle Events (Service Started/Stopped)

Services publish events when they start up or shut down to enable service monitoring and coordination.

**Service Started Event**
When a service successfully starts, it publishes a started event.

- **Channel:** `events/service/started`
- **Message Type:** `service.started`
- **Message:**
```json
{
  "envelope": {
    "id": "msg-1234-5678-90ab-cdef",
    "timestamp": "2025-07-18T10:00:00.123Z",
    "source": { "service": "tcp_manager_service", "instance": "tm-01", "version": "1.0.0" },
    "schema_version": "v1",
    "type": "service.started"
  },
  "payload": {
    "service_name": "tcp_manager_service",
    "instance_id": "tm-01",
    "version": "1.0.0",
    "timestamp": "2025-07-18T10:00:00.123Z",
    "pid": 12345
  }
}
```

**Service Stopped Event**
When a service shuts down, it publishes a stopped event.

- **Channel:** `events/service/stopped`
- **Message Type:** `service.stopped`
- **Message:**
```json
{
  "envelope": {
    "id": "msg-1234-5678-90ab-cdef",
    "timestamp": "2025-07-18T11:00:00.123Z",
    "source": { "service": "tcp_manager_service", "instance": "tm-01", "version": "1.0.0" },
    "schema_version": "v1",
    "type": "service.stopped"
  },
  "payload": {
    "service_name": "tcp_manager_service",
    "instance_id": "tm-01",
    "reason": "signal",
    "timestamp": "2025-07-18T11:00:00.123Z"
  }
}
```

**Monitoring Services:**
- Services that monitor lifecycle (e.g., `terminal_dashboard`) subscribe to `events/service/*`
- Service payloads are defined in `/schemas/service.started/v1.json` and `/schemas/service.stopped/v1.json`
- Services should publish the stopped event before shutting down Redis connections

---

## 6. Error Handling Specification

Errors MUST be communicated using the standardized structured error object. The `error` field in a response payload is **`null` on success** and contains the error object on failure.

### The Structured Error Object

```json
{
  "code": "E_CATEGORY_DETAIL",
  "message": "A human-readable summary of the error.",
  "details": {
    "key": "Machine-parsable context about the error."
  }
}
```

### Standard Error Codes (Examples)

This is not an exhaustive list, but establishes the naming convention.

| Code | Meaning | Example Details |
| :--- | :--- | :--- |
| **`E_VALIDATION_FAILED`** | Incoming message failed JSON Schema validation. | `{"field": "payload.timeout_ms", "reason": "must be >= 0"}` |
| **`E_DEVICE_CONNECTION`** | Could not connect to the physical device. | `{"address": "192.168.1.50", "reason": "Connection refused"}` |
| **`E_DEVICE_TIMEOUT`** | Timed out waiting for a response from the device. | `{"timeout_ms": 5000}` |
| **`E_INVALID_PARAM`** | A parameter in the command payload is invalid. | `{"param": "state", "allowed": ["ON", "OFF"], "received": "ENABLED"}` |
| **`E_AUTH_FAILED`** | The JWT in the auth block is invalid or expired. | `{"reason": "Token expired"}` |
| **`E_UNAUTHORIZED`** | The identity is valid, but not permitted to perform the action. | `{"permission_needed": "device:command"}` |
| **`E_INTERNAL_SERVICE`** | An unexpected error occurred inside the service. | `{"error": "Database connection lost", "stack_trace": "..."}` |

---

## 7. Implementation Mandates

1.  **Schema-First Development:** When creating a new message type or version, the JSON Schema file MUST be created or updated first.
2.  **Generate Data Structures:** Service developers SHOULD use tools to generate Go structs from the JSON Schemas to ensure compile-time safety and adherence to the contract.
3.  **Mandatory Incoming Validation:** Every service MUST validate the payload of every message it consumes against the correct schema version.
4.  **Mandatory Outgoing Validation:** All CI pipelines MUST include a step to validate that messages produced by a service conform to the schemas.
5.  **Use the `reply_to` Pattern:** For any request/response interaction, the requester MUST provide a `reply_to` channel and the responder MUST use it. Hardcoding response channels is forbidden.

## 8. Schema Directory Structure

The `/schemas` directory will be organized as follows:

```
/schemas/
├── common/
│   └── error.v1.json
├── device.command.request/
│   └── v1.json
├── device.command.response/
│   └── v1.json
├── device.discovered.event/
│   └── v1.json
└── ... (other message types)
```
