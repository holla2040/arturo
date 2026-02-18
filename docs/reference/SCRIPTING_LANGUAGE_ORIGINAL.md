# Arturo Scripting Language Guide

## Table of Contents

1. [Introduction](#introduction)
2. [Getting Started](#getting-started)
3. [Language Fundamentals](#language-fundamentals)
4. [Device Communication](#device-communication)
5. [Variables and Data Types](#variables-and-data-types)
6. [Control Flow](#control-flow)
7. [Functions and Libraries](#functions-and-libraries)
8. [Advanced Features](#advanced-features)
9. [Best Practices](#best-practices)
10. [Command Reference](#command-reference)
11. [Examples](#examples)
12. [Troubleshooting](#troubleshooting)

## Introduction

The Arturo scripting language is a domain-specific language designed for industrial test automation. It provides a human-readable syntax for controlling test equipment, collecting data, and implementing complex test procedures.

### Key Features

- **Simple, readable syntax** - Write tests that read like procedures
- **Device abstraction** - Unified interface for TCP/IP, serial, and USB devices
- **Built-in test constructs** - Native support for test cases, assertions, and reporting
- **Powerful control flow** - Loops, conditionals, and error handling
- **Extensible** - Functions, libraries, and custom commands
- **Real-time execution** - Parallel operations and event-driven testing

### File Extensions

- `.art` - Arturo script files
- `.artlib` - Arturo library files
- `.arturo` - Alternative extension

## Getting Started

### Hello World

```arturo
# hello_world.art
TEST "Hello World"
    LOG INFO "Hello, Arturo!"
    PASS "Test completed successfully"
ENDTEST
```

### Running Scripts

```bash
# Using the script executor
cd src/components/20_script_executor
./script_executor hello_world.art

# With parameters
./script_executor -script test.art -var "voltage=5.0" -var "current=0.5"
```

## Language Fundamentals

### Comments

```arturo
# This is a single-line comment

# Multi-line comments are done with
# multiple single-line comments
```

### Test Structure

```arturo
TEST "Test Name"
    # Setup phase (parser implementation syntax)
    CONNECT device TCP "192.168.1.100:5025"
    
    # Test execution
    # ... test commands ...
    
    # Cleanup
    DISCONNECT device
    
    # Test result
    PASS "Test passed"
    # or
    FAIL "Test failed: reason"
ENDTEST
```

### Test Suites

```arturo
SUITE "Production Test Suite"
    SETUP
        # Global setup for all tests (parser implementation syntax)
        SET test_id "SUITE-" + NOW()
    ENDSETUP
    
    TEST "Power Test"
        # Individual test
    ENDTEST
    
    TEST "Frequency Test"
        # Another test
    ENDTEST
    
    TEARDOWN
        # Global cleanup
        GENERATE_REPORT test_id
    ENDTEARDOWN
ENDSUITE
```

## Device Communication

### Connection Management

```arturo
# TCP/IP connection (SCPI devices) - parser implementation syntax
CONNECT scope TCP "192.168.1.100:5025"
CONNECT dmm TCP "10.0.0.50:5025"

# Serial connection
CONNECT relay SERIAL "/dev/ttyUSB0" 9600
CONNECT pump SERIAL "COM3" 9600 8 N 1

# Disconnect
DISCONNECT scope
DISCONNECT ALL  # Disconnect all devices
```

### Sending Commands

```arturo
# Simple command
SEND scope "*RST"

# Command with variables (parser implementation syntax)
SET voltage 5.0
SEND psu "VOLT " + voltage

# Multiple commands
SEND scope "*RST; :CHAN1:DISP ON; :TRIG:MODE NORM"
```

### Receiving Responses

```arturo
# Query with response
QUERY scope "*IDN?" device_id

# Query with timeout
QUERY dmm "MEAS:VOLT?" voltage TIMEOUT 5000

# Binary data query
QUERY_BINARY scope "WAV:DATA?" waveform_data
```

### Relay Control

```arturo
# Control relay channels
RELAY relay_board SET 1 ON
RELAY relay_board SET 2 OFF
RELAY relay_board TOGGLE 3

# Get relay state
RELAY relay_board GET 1 relay_state
```

## Variables and Data Types

### Variable Declaration

```arturo
# Automatic type inference (parser implementation syntax)
SET voltage 5.0          # Float
SET device_name "DMM"    # String
SET count 10             # Integer
SET enabled true         # Boolean

# Arrays
SET voltages [1.0, 2.5, 3.3, 5.0]
SET devices ["DMM", "PSU", "SCOPE"]

# Dictionaries/Maps
SET config {"voltage": 5.0, "current": 0.5, "enabled": true}
SET results {}  # Empty dictionary
```

### Variable Usage

```arturo
# String concatenation (parser implementation syntax)
LOG INFO "Voltage is " + voltage + "V"

# Array access (using bare variables)
SET first_voltage voltages[0]
SET last_voltage voltages[-1]

# Dictionary access
SET v config["voltage"]
SET config["power"] v * config["current"]

# Array operations
APPEND voltages 12.0
EXTEND voltages [15.0, 24.0]
SET length LENGTH(voltages)
```

### Type Conversion

```arturo
# String to number (parser implementation syntax)
SET str_value "3.14"
SET num_value FLOAT(str_value)

# Number to string
SET voltage 5.0
SET voltage_str STRING(voltage)

# Boolean conversion
SET bool_val BOOL("true")  # true
SET bool_val BOOL(1)       # true
SET bool_val BOOL("")      # false
```

## Control Flow

### Conditional Statements

```arturo
# Basic if statement (parser implementation syntax)
IF voltage > 5.0
    LOG WARN "Voltage too high"
ENDIF

# If-else
IF voltage < 4.9
    LOG ERROR "Voltage too low"
    FAIL "Undervoltage"
ELSE
    LOG INFO "Voltage OK"
ENDIF

# If-elseif-else
IF voltage < 4.9
    SET status "LOW"
ELSEIF voltage > 5.1
    SET status "HIGH"
ELSE
    SET status "OK"
ENDIF

# Logical operators (C-style preferred)
IF voltage >= 4.9 && voltage <= 5.1
    PASS "Voltage within tolerance"
ENDIF

IF error_count > 0 || timeout_occurred
    FAIL "Test failed"
ENDIF
```

### Loops

```arturo
# Counted loop (parser implementation syntax)
LOOP 10 TIMES
    QUERY dmm "MEAS:VOLT?" voltage
    LOG INFO "Reading " + LOOP_INDEX + ": " + voltage + "V"
ENDLOOP

# Loop with variable
SET iterations 5
LOOP iterations TIMES AS i
    LOG INFO "Iteration " + i + " of " + iterations
ENDLOOP

# While loop
SET voltage 0
WHILE voltage < 4.9
    SEND psu "VOLT:UP"
    DELAY 500
    QUERY psu "MEAS:VOLT?" voltage
ENDWHILE

# For-each loop
SET test_voltages [1.0, 2.5, 3.3, 5.0]
FOREACH voltage IN test_voltages
    SEND psu "VOLT " + voltage
    DELAY 1000
    # Test at this voltage
ENDFOREACH

# For-each with index
FOREACH device IN devices AS index
    LOG INFO "Device " + index + ": " + device
ENDFOREACH
```

### Loop Control

```arturo
# Break statement (parser implementation syntax)
LOOP 100 TIMES
    QUERY dmm "MEAS:VOLT?" voltage
    IF voltage > 5.0
        LOG INFO "Target voltage reached"
        BREAK
    ENDIF
    DELAY 100
ENDLOOP

# Continue statement
FOREACH voltage IN test_voltages
    IF voltage < 1.0
        LOG WARN "Skipping low voltage"
        CONTINUE
    ENDIF
    # Process valid voltage
ENDFOREACH
```

## Functions and Libraries

### Function Definition

```arturo
# Basic function
FUNCTION say_hello()
    LOG INFO "Hello from function!"
ENDFUNCTION

# Function with parameters (parser implementation syntax)
FUNCTION measure_voltage(device, channel)
    SEND device "ROUT:CLOS (@" + channel + ")"
    DELAY 100
    QUERY device "MEAS:VOLT?" voltage
    RETURN voltage
ENDFUNCTION

# Function with default parameters
FUNCTION configure_dmm(device, range, resolution)
    # Set defaults if not provided
    IF range == null
        SET range "AUTO"
    ENDIF
    IF resolution == null
        SET resolution "DEF"
    ENDIF
    SEND device "CONF:VOLT:DC " + range + "," + resolution
ENDFUNCTION
```

### Function Calls

```arturo
# Call simple function
CALL say_hello()

# Call with return value (parser implementation syntax)
SET v1 CALL measure_voltage(dmm, 101)
SET v2 CALL measure_voltage(dmm, 102)

# Call with parameters
CALL configure_dmm(dmm, "10", "0.001")
```

### Libraries

```arturo
# test_utilities.artlib
LIBRARY "TestUtilities"

FUNCTION validate_voltage(measured, expected, tolerance_percent)
    SET tolerance expected * tolerance_percent / 100
    SET min expected - tolerance
    SET max expected + tolerance
    
    IF measured >= min && measured <= max
        RETURN true
    ELSE
        LOG ERROR "Voltage " + measured + "V outside range [" + min + ", " + max + "]"
        RETURN false
    ENDIF
ENDFUNCTION

FUNCTION calculate_statistics(values)
    SET count LENGTH(values)
    SET sum 0
    SET min values[0]
    SET max values[0]
    
    FOREACH val IN values
        SET sum sum + val
        IF val < min
            SET min val
        ENDIF
        IF val > max
            SET max val
        ENDIF
    ENDFOREACH
    
    SET avg sum / count
    RETURN {"min": min, "max": max, "avg": avg, "count": count}
ENDFUNCTION

ENDLIBRARY
```

### Using Libraries

```arturo
# Import library
IMPORT "test_utilities.artlib"

# Use library functions (parser implementation syntax)
SET valid CALL validate_voltage(4.95, 5.0, 2.0)
SET stats CALL calculate_statistics([1.0, 2.0, 3.0, 4.0, 5.0])
```

## Advanced Features

### Error Handling

```arturo
# Try-catch block
TRY
    CONNECT device TCP "192.168.1.100:5025"
    QUERY device "*IDN?" device_id TIMEOUT 5000
CATCH error
    LOG ERROR "Device communication failed: " + error
    SET device_id "Unknown"
ENDTRY

# Try-catch-finally
TRY
    # Risky operation
    SEND device "SYST:ERR?"
    RECEIVE device error_msg TIMEOUT 1000
CATCH error
    LOG ERROR "Error check failed: ${error}"
FINALLY
    # Always executed
    LOG INFO "Error check complete"
ENDTRY
```

### Parallel Execution

```arturo
# Execute commands in parallel
PARALLEL
    QUERY dmm1 "MEAS:VOLT?" voltage1
    QUERY dmm2 "MEAS:VOLT?" voltage2
    QUERY dmm3 "MEAS:VOLT?" voltage3
ENDPARALLEL

# All queries complete before continuing
LOG INFO "V1=" + voltage1 + ", V2=" + voltage2 + ", V3=" + voltage3

# Parallel with timeout
PARALLEL TIMEOUT 10000
    CONNECT dev1 TCP "192.168.1.101:5025"
    CONNECT dev2 TCP "192.168.1.102:5025"
    CONNECT dev3 TCP "192.168.1.103:5025"
ENDPARALLEL
```

### Event Handling

```arturo
# Wait for event
WAIT_FOR EVENT "device/trigger" AS event_data TIMEOUT 5000
    LOG INFO "Trigger received: " + event_data
    # Process trigger
ENDWAIT

# Multiple events
WAIT_FOR EVENTS
    EVENT "device/error" AS error
        LOG ERROR "Device error: " + error
        FAIL "Device error occurred"
    EVENT "device/complete" AS result
        LOG INFO "Operation complete: " + result
        BREAK  # Exit wait
    TIMEOUT 30000
        FAIL "Operation timeout"
ENDWAIT
```

### Assertions

```arturo
# Basic assertions (parser implementation syntax)
ASSERT voltage > 0 "Voltage must be positive"
ASSERT voltage < 50 "Voltage too high"

# Complex assertions
ASSERT voltage >= 4.9 && voltage <= 5.1 "Voltage out of tolerance"
ASSERT EXISTS config["voltage"] "Voltage not configured"
ASSERT TYPE(voltage) == "float" "Voltage must be a number"
```

### Debug Features

```arturo
# Enable debug mode
SET_DEBUG true

# Debug logging (parser implementation syntax)
LOG DEBUG "Current voltage: " + voltage

# Trace variable changes
TRACE voltage, current, power

# Breakpoints
BREAKPOINT "Check variables before calculation"

# Performance timing
TIMER_START "measurement_cycle"
# ... operations ...
SET elapsed TIMER_STOP("measurement_cycle")
LOG INFO "Cycle took " + elapsed + "ms"
```

## Best Practices

### Script Organization

```arturo
# script_template.art

# File header
# Description: Production test for Model XYZ
# Author: Engineering Team
# Date: 2024-01-15
# Version: 1.0

# Imports
IMPORT "common_functions.artlib"
IMPORT "device_profiles.artlib"

# Constants (parser implementation syntax)
CONST VOLTAGE_NOMINAL 5.0
CONST VOLTAGE_TOLERANCE 0.1
CONST MEASUREMENT_COUNT 10

# Global variables
SET test_id "TEST-" + NOW()
SET error_count 0

# Main test
TEST "Production Test - Model XYZ"
    # Setup
    CALL setup_test_environment()
    
    # Execute test steps
    CALL test_power_supply()
    CALL test_analog_inputs()
    CALL test_digital_outputs()
    
    # Cleanup
    CALL cleanup_test_environment()
    
    # Results
    IF ${error_count} == 0
        PASS "All tests passed"
    ELSE
        FAIL "${error_count} errors occurred"
    ENDIF
ENDTEST
```

### Error Handling Best Practices

```arturo
# Defensive programming
FUNCTION safe_voltage_measurement(device, max_retries)
    # Set default if not provided
    IF max_retries == null
        SET max_retries 3
    ENDIF
    
    SET attempt 0
    SET success false
    SET voltage 0.0
    
    WHILE attempt < max_retries && !success
        TRY
            QUERY device "MEAS:VOLT?" voltage TIMEOUT 5000
            SET success true
        CATCH error
            SET attempt attempt + 1
            LOG WARN "Measurement attempt " + attempt + " failed: " + error
            IF attempt < max_retries
                DELAY 1000  # Wait before retry
            ENDIF
        ENDTRY
    ENDWHILE
    
    IF !success
        LOG ERROR "Failed to measure voltage after " + max_retries + " attempts"
        RETURN {"success": false, "voltage": 0.0}
    ENDIF
    
    RETURN {"success": true, "voltage": voltage}
ENDFUNCTION
```

### Performance Optimization

```arturo
# Batch operations (parser implementation syntax)
BATCH_START
    SEND analyzer "FREQ:STAR 1E6"
    SEND analyzer "FREQ:STOP 1E9"
    SEND analyzer "BAND 10E3"
    SEND analyzer "AVER:COUN 10"
BATCH_END

# Efficient data collection
SET measurements []
RESERVE measurements 1000  # Pre-allocate array

# Use binary transfers for large data
SEND device "FORM:DATA REAL,32"
QUERY_BINARY device "TRAC:DATA?" trace_data
```

## Command Reference

### Device Commands

| Command | Description | Syntax |
|---------|-------------|--------|
| CONNECT | Establish device connection | `CONNECT device_id TYPE "address:port" [options]` |
| DISCONNECT | Close device connection | `DISCONNECT device_id` |
| SEND | Send command to device | `SEND device_id "command"` |
| QUERY | Send command and receive response | `QUERY device_id "command" variable [TIMEOUT ms]` |
| QUERY_BINARY | Query binary data | `QUERY_BINARY device_id "command" variable` |
| RELAY | Control relay channels | `RELAY device_id SET/GET/TOGGLE channel [state]` |

### Variable Commands

| Command | Description | Syntax |
|---------|-------------|--------|
| SET | Assign variable | `SET variable expression` |
| CONST | Define constant | `CONST name value` |
| GLOBAL | Declare global variable | `GLOBAL variable_name` |
| DELETE | Remove variable | `DELETE variable_name` |

### Control Flow Commands

| Command | Description | Syntax |
|---------|-------------|--------|
| IF/ENDIF | Conditional execution | `IF condition ... ENDIF` |
| LOOP/ENDLOOP | Counted or conditional loop | `LOOP count TIMES ... ENDLOOP` |
| WHILE/ENDWHILE | While loop | `WHILE condition ... ENDWHILE` |
| FOREACH/ENDFOREACH | Iterate over collection | `FOREACH item IN collection ... ENDFOREACH` |
| BREAK | Exit loop | `BREAK` |
| CONTINUE | Skip to next iteration | `CONTINUE` |

### Function Commands

| Command | Description | Syntax |
|---------|-------------|--------|
| FUNCTION | Define function | `FUNCTION name(params) ... ENDFUNCTION` |
| CALL | Call function | `CALL function_name(args)` |
| RETURN | Return from function | `RETURN [value]` |

### Test Commands

| Command | Description | Syntax |
|---------|-------------|--------|
| TEST/ENDTEST | Define test case | `TEST "name" ... ENDTEST` |
| SUITE/ENDSUITE | Define test suite | `SUITE "name" ... ENDSUITE` |
| PASS | Mark test as passed | `PASS "message"` |
| FAIL | Mark test as failed | `FAIL "message"` |
| SKIP | Skip test | `SKIP "reason"` |

### Utility Commands

| Command | Description | Syntax |
|---------|-------------|--------|
| LOG | Write to log | `LOG LEVEL "message"` |
| DELAY | Wait for specified time | `DELAY milliseconds` |
| IMPORT | Import library | `IMPORT "filename"` |
| INCLUDE | Include script file | `INCLUDE "filename"` |
| ASSERT | Validate condition | `ASSERT condition "message"` |
| TIMER_START | Start timer | `TIMER_START "name"` |
| TIMER_STOP | Stop timer and get elapsed time | `TIMER_STOP("name")` |

## Examples

### Complete Test Script

```arturo
# production_test.art - Complete production test example

IMPORT "test_utilities.artlib"

# Test configuration (parser implementation syntax)
CONST VOLTAGE_SPEC 5.0
CONST VOLTAGE_TOLERANCE 2.0  # percent
CONST CURRENT_LIMIT 1.0
CONST TEST_POINTS [1.0, 2.5, 3.3, 5.0, 9.0, 12.0]

TEST "Power Supply Production Test"
    # Initialize
    SET serial_number INPUT("Enter serial number: ")
    SET test_id "PSU-" + serial_number + "-" + NOW()
    SET results []
    
    LOG INFO "Starting test ${test_id}"
    
    # Connect to equipment
    TRY
        CONNECT psu TCP "192.168.1.100:5025"
        CONNECT dmm TCP "192.168.1.101:5025"
        CONNECT load TCP "192.168.1.102:5025"
    CATCH error
        FAIL "Failed to connect to test equipment: " + error
    ENDTRY
    
    # Configure instruments
    SEND psu "*RST"
    SEND dmm "*RST"
    SEND load "*RST"
    DELAY 2000
    
    # Test each voltage level
    FOREACH voltage IN TEST_POINTS
        LOG INFO "Testing " + voltage + "V output"
        
        # Set output voltage
        SEND psu "VOLT " + voltage
        SEND psu "CURR " + CURRENT_LIMIT
        SEND psu "OUTP ON"
        DELAY 1000
        
        # Measure with different loads
        SET load_currents [0.1, 0.5, 1.0]
        FOREACH load_current IN load_currents
            # Set load
            SEND load "CURR " + load_current
            SEND load "INP ON"
            DELAY 500
            
            # Measure voltage
            QUERY dmm "MEAS:VOLT?" measured_voltage
            
            # Validate
            SET valid CALL validate_voltage(measured_voltage, voltage, VOLTAGE_TOLERANCE)
            
            # Store result
            APPEND results {
                "voltage_set": voltage,
                "load_current": load_current,
                "voltage_measured": measured_voltage,
                "passed": valid
            }
            
            # Log result
            IF valid
                LOG PASS "Voltage " + voltage + "V @ " + load_current + "A: " + measured_voltage + "V OK"
            ELSE
                LOG FAIL "Voltage " + voltage + "V @ " + load_current + "A: " + measured_voltage + "V FAIL"
            ENDIF
        ENDFOREACH
        
        # Turn off load
        SEND load "INP OFF"
    ENDFOREACH
    
    # Cleanup
    SEND psu "OUTP OFF"
    SEND load "INP OFF"
    DISCONNECT ALL
    
    # Generate report
    SET stats CALL analyze_results(results)
    SAVE_JSON results "results/" + test_id + ".json"
    GENERATE_REPORT test_id stats
    
    # Final result
    IF stats["pass_rate"] == 100
        PASS "All tests passed"
    ELSE
        FAIL "Pass rate: " + stats["pass_rate"] + "%"
    ENDIF
ENDTEST
```

### Parallel Device Testing

```arturo
# parallel_test.art - Test multiple devices simultaneously

TEST "Multi-Channel Voltage Test"
    # Connect to multiple channels (parser implementation syntax)
    SET channels [1, 2, 3, 4]
    SET dmm_addresses [
        "192.168.1.101",
        "192.168.1.102", 
        "192.168.1.103",
        "192.168.1.104"
    ]
    
    # Connect all DMMs in parallel
    PARALLEL
        FOREACH addr IN dmm_addresses AS i
            CONNECT dmm + i TCP addr + ":5025"
        ENDFOREACH
    ENDPARALLEL
    
    # Configure all DMMs
    PARALLEL
        FOREACH i IN channels
            SEND dmm + i "CONF:VOLT:DC 10"
        ENDFOREACH
    ENDPARALLEL
    
    DELAY 1000
    
    # Take simultaneous measurements
    SET measurements {}
    PARALLEL
        FOREACH i IN channels
            QUERY dmm + i "MEAS:VOLT:DC?" voltage
            SET measurements["ch" + i] voltage
        ENDFOREACH
    ENDPARALLEL
    
    # Analyze results
    FOREACH i IN channels
        SET voltage measurements["ch" + i]
        LOG INFO "Channel " + i + ": " + voltage + "V"
        
        IF voltage < 4.9 || voltage > 5.1
            LOG WARN "Channel " + i + " out of tolerance"
        ENDIF
    ENDFOREACH
    
    # Cleanup
    PARALLEL
        FOREACH i IN channels
            DISCONNECT dmm + i
        ENDFOREACH
    ENDPARALLEL
    
    PASS "Multi-channel test complete"
ENDTEST
```

### Event-Driven Testing

```arturo
# event_driven_test.art - React to external events

TEST "Triggered Measurement Test"
    CONNECT scope TCP "192.168.1.100:5025"
    CONNECT signal_gen TCP "192.168.1.101:5025"
    
    # Configure scope for single trigger
    SEND scope "*RST"
    SEND scope "TRIG:MODE SING"
    SEND scope "TRIG:SOUR EXT"
    SEND scope "TRIG:LEV 2.5"
    
    # Arm the trigger
    SEND scope "INIT"
    LOG INFO "Waiting for trigger..."
    
    # Start signal generator burst
    SEND signal_gen "BURS:NCYC 10"
    SEND signal_gen "BURS:STAT ON"
    
    # Wait for trigger event (parser implementation syntax)
    SET triggered false
    WAIT_FOR EVENT "scope/triggered" AS trigger_data TIMEOUT 10000
        SET triggered true
        LOG INFO "Trigger received at " + trigger_data["timestamp"]
        
        # Capture and analyze waveform
        QUERY_BINARY scope "WAV:DATA?" waveform
        SET analysis CALL analyze_waveform(waveform)
        
        LOG INFO "Peak amplitude: " + analysis["peak"] + "V"
        LOG INFO "Frequency: " + analysis["frequency"] + "Hz"
        
        # Validate results
        ASSERT analysis["peak"] > 4.5 && analysis["peak"] < 5.5 "Peak amplitude out of range"
        ASSERT analysis["frequency"] > 990 && analysis["frequency"] < 1010 "Frequency out of range"
        
    ENDWAIT
    
    IF !triggered
        FAIL "No trigger received within timeout"
    ELSE
        PASS "Triggered measurement successful"
    ENDIF
    
    DISCONNECT ALL
ENDTEST
```

## Troubleshooting

### Common Errors

#### Syntax Errors

```arturo
# Missing ENDIF
IF voltage > 5.0
    LOG ERROR "Voltage too high"
# ERROR: Expected ENDIF

# Mismatched quotes
SET message "This is a test'
# ERROR: Unterminated string

# Invalid variable syntax
SET value $voltage  # Missing proper syntax
# ERROR: Invalid variable reference
```

#### Runtime Errors

```arturo
# Undefined variable (parser implementation syntax)
LOG INFO "Voltage: " + undefined_var
# ERROR: Variable 'undefined_var' is not defined

# Array index out of bounds
SET values [1, 2, 3]
SET fourth values[3]
# ERROR: Index 3 out of bounds for array of length 3

# Type mismatch
SET text "hello"
SET result text + 5
# ERROR: Cannot add string and number
```

### Debug Techniques

```arturo
# Enable verbose logging
SET_LOG_LEVEL DEBUG

# Print variable state (parser implementation syntax)
FUNCTION debug_state()
    LOG DEBUG "=== Variable State ==="
    LOG DEBUG "voltage: " + voltage
    LOG DEBUG "current: " + current
    LOG DEBUG "power: " + power
    LOG DEBUG "==================="
ENDFUNCTION

# Use assertions for validation
ASSERT voltage != null "Voltage not initialized"
ASSERT TYPE(voltage) == "float" "Voltage must be numeric"

# Conditional debug output
IF DEBUG_MODE
    LOG DEBUG "Entering critical section"
    CALL debug_state()
ENDIF
```

### Performance Issues

```arturo
# Slow: Individual commands
LOOP 100 TIMES
    SEND device "MEAS:VOLT?"
    RECEIVE device voltage
ENDLOOP

# Fast: Batch operations
SEND device "SAMP:COUN 100"
SEND device "INIT"
QUERY device "FETC?" all_voltages

# Slow: Repeated connections
LOOP 10 TIMES
    CONNECT device TCP "192.168.1.100:5025"
    QUERY device "MEAS?" value
    DISCONNECT device
ENDLOOP

# Fast: Reuse connection (parser implementation syntax)
CONNECT device TCP "192.168.1.100:5025"
LOOP 10 TIMES
    QUERY device "MEAS?" value
ENDLOOP
DISCONNECT device
```

## Integration with Project Arturo

The Arturo scripting language integrates seamlessly with all Project Arturo components:

- **Script Parser (Component 19)**: Validates and parses scripts into executable form
- **Script Executor (Component 20)**: Executes parsed scripts with real-time control
- **Variable System (Component 21)**: Manages script variables and expressions
- **Device Services**: Provides device communication for CONNECT/SEND/QUERY commands
- **Data Services**: Handles data storage and retrieval operations
- **Web Interface**: Allows script editing and execution through the browser
- **Discord Integration**: Enables remote script execution via Discord commands

For more information on specific components, refer to the component documentation in the `src/components/` directory.

## Conclusion

The Arturo scripting language provides a powerful yet approachable way to automate industrial test procedures. Its design prioritizes readability and maintainability while offering the features needed for complex test scenarios.

For additional examples and advanced usage patterns, explore the example scripts in the component directories and the learning guide modules.