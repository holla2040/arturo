# Operator Guide

How to run tests from the Terminal UI.

---

## Login

Open the Terminal in a browser. Enter your Employee ID and Name, then click **Login**.

---

## Station Overview

After login you see the station grid — one card per connected station showing its status. Click a station card to open its detail view.

---

## Running a Test

### Prerequisites

- At least one station must be reporting (visible in the station grid).
- An open RMA must exist for the pump being tested. If you don't have one, click **New RMA** in the header and fill in the RMA number, pump serial, customer, model, and any notes.

### Start a Test

1. Click a station card to open its detail view.
2. The station must show **idle** or **online** status. If it does, you'll see a **Start Test** button under Test Controls.
3. Click **Start Test**. A dialog appears with two dropdowns:
   - **RMA** — select the open RMA for this pump.
   - **Script** — select the `.art` test script to run (e.g. `pump_status`, `pump_cycle`, `regen_temp_monitor`).
4. Click **Start**.

### During a Test

While a test is running, the station status changes to **testing** and the controls change to:

- **Pause** — pause the test. You can resume later.
- **Terminate** — stop the test gracefully. You'll be asked for a reason. All collected data is preserved.
- **Abort** — stop the test immediately.

Temperature data is charted in real time. Use the time window buttons (1h, 2h, 4h, 8h, Full) to adjust the chart range, or **Export CSV** to download the temperature data.

---

## Validating a Script

Before running a script on a live station, validate it from the command line:

```bash
cd tools/engine && ./engine validate <script.art>
```

This parses the script and reports any syntax errors as structured JSON — no hardware or Redis connection needed. Fix any errors before loading the script in the Terminal.

---

## Creating an RMA

1. Click **New RMA** in the header (or go to **RMAs** and click **New RMA**).
2. Fill in:
   - **RMA Number** — your tracking number (e.g. `RMA-2024-001`).
   - **Pump Serial Number** — the serial number on the pump.
   - **Customer Name** — who sent the pump.
   - **Pump Model** — the pump model (e.g. `CT-8`).
   - **Notes** — optional.
3. Click **Create RMA**.

The RMA must be open before you can start a test against it.

---

## Emergency Stop

If the E-stop is triggered (physical button or software), a red **EMERGENCY STOP** banner appears across the top of the screen showing the reason, who initiated it, and when. All test operations halt. The header badge changes from **SAFE** to reflect the E-stop state.
