package result

import (
	"testing"
)

// ---------------------------------------------------------------------------
// NewCollector
// ---------------------------------------------------------------------------

func TestNewCollector(t *testing.T) {
	c := NewCollector("test.art")
	if c == nil {
		t.Fatal("NewCollector returned nil")
	}
	if c.scriptPath != "test.art" {
		t.Errorf("scriptPath = %q, want %q", c.scriptPath, "test.art")
	}
	if c.startTime.IsZero() {
		t.Error("startTime should not be zero")
	}
}

// ---------------------------------------------------------------------------
// Single test outcomes
// ---------------------------------------------------------------------------

func TestSingleTestPass(t *testing.T) {
	c := NewCollector("pass.art")
	c.RecordTestStart("t1")
	c.RecordTestPass("t1", "all good")
	report := c.Finalize()

	if len(report.Tests) != 1 {
		t.Fatalf("expected 1 standalone test, got %d", len(report.Tests))
	}
	tr := report.Tests[0]
	if tr.Status != "passed" {
		t.Errorf("status = %q, want %q", tr.Status, "passed")
	}
	if report.Summary.Total != 1 || report.Summary.Passed != 1 {
		t.Errorf("summary = %+v, want total=1 passed=1", report.Summary)
	}
}

func TestSingleTestFail(t *testing.T) {
	c := NewCollector("fail.art")
	c.RecordTestStart("t1")
	c.RecordTestFail("t1", "value mismatch")
	report := c.Finalize()

	if len(report.Tests) != 1 {
		t.Fatalf("expected 1 standalone test, got %d", len(report.Tests))
	}
	if report.Tests[0].Status != "failed" {
		t.Errorf("status = %q, want %q", report.Tests[0].Status, "failed")
	}
	if report.Summary.Failed != 1 {
		t.Errorf("Summary.Failed = %d, want 1", report.Summary.Failed)
	}
}

func TestSingleTestSkip(t *testing.T) {
	c := NewCollector("skip.art")
	c.RecordTestStart("t1")
	c.RecordTestSkip("t1", "not applicable")
	report := c.Finalize()

	if len(report.Tests) != 1 {
		t.Fatalf("expected 1 standalone test, got %d", len(report.Tests))
	}
	if report.Tests[0].Status != "skipped" {
		t.Errorf("status = %q, want %q", report.Tests[0].Status, "skipped")
	}
	if report.Summary.Skipped != 1 {
		t.Errorf("Summary.Skipped = %d, want 1", report.Summary.Skipped)
	}
}

func TestSingleTestError(t *testing.T) {
	c := NewCollector("error.art")
	c.RecordTestStart("t1")
	c.RecordTestError("t1", "device unreachable")
	report := c.Finalize()

	if len(report.Tests) != 1 {
		t.Fatalf("expected 1 standalone test, got %d", len(report.Tests))
	}
	if report.Tests[0].Status != "error" {
		t.Errorf("status = %q, want %q", report.Tests[0].Status, "error")
	}
	if report.Summary.Errors != 1 {
		t.Errorf("Summary.Errors = %d, want 1", report.Summary.Errors)
	}
}

// ---------------------------------------------------------------------------
// Multiple tests
// ---------------------------------------------------------------------------

func TestMultipleTests(t *testing.T) {
	c := NewCollector("multi.art")

	c.RecordTestStart("pass-test")
	c.RecordTestPass("pass-test", "ok")

	c.RecordTestStart("fail-test")
	c.RecordTestFail("fail-test", "bad")

	c.RecordTestStart("skip-test")
	c.RecordTestSkip("skip-test", "skipped")

	report := c.Finalize()

	if report.Summary.Total != 3 {
		t.Errorf("Total = %d, want 3", report.Summary.Total)
	}
	if report.Summary.Passed != 1 {
		t.Errorf("Passed = %d, want 1", report.Summary.Passed)
	}
	if report.Summary.Failed != 1 {
		t.Errorf("Failed = %d, want 1", report.Summary.Failed)
	}
	if report.Summary.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", report.Summary.Skipped)
	}
}

// ---------------------------------------------------------------------------
// Assertions and commands within tests
// ---------------------------------------------------------------------------

func TestAssertionsRecorded(t *testing.T) {
	c := NewCollector("assert.art")
	c.RecordTestStart("t1")
	c.RecordAssertion("t1", true, "1 == 1")
	c.RecordAssertion("t1", false, "2 == 3")
	c.RecordTestPass("t1", "done")
	report := c.Finalize()

	if len(report.Tests) != 1 {
		t.Fatalf("expected 1 test, got %d", len(report.Tests))
	}
	if len(report.Tests[0].Assertions) != 2 {
		t.Fatalf("expected 2 assertions, got %d", len(report.Tests[0].Assertions))
	}
	if !report.Tests[0].Assertions[0].Passed {
		t.Error("first assertion should be passed")
	}
	if report.Tests[0].Assertions[1].Passed {
		t.Error("second assertion should not be passed")
	}
}

func TestCommandsRecorded(t *testing.T) {
	c := NewCollector("cmd.art")
	c.RecordTestStart("t1")
	c.RecordCommand("t1", "dmm-1", "*IDN?", true, "Keithley,2000", 42)
	c.RecordTestPass("t1", "done")
	report := c.Finalize()

	if len(report.Tests) != 1 {
		t.Fatalf("expected 1 test, got %d", len(report.Tests))
	}
	if len(report.Tests[0].Commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(report.Tests[0].Commands))
	}
	cmd := report.Tests[0].Commands[0]
	if cmd.DeviceID != "dmm-1" {
		t.Errorf("DeviceID = %q, want %q", cmd.DeviceID, "dmm-1")
	}
	if cmd.Response != "Keithley,2000" {
		t.Errorf("Response = %q, want %q", cmd.Response, "Keithley,2000")
	}
	if !cmd.Success {
		t.Error("command should be successful")
	}
}

// ---------------------------------------------------------------------------
// Suite lifecycle
// ---------------------------------------------------------------------------

func TestSuiteWithTests(t *testing.T) {
	c := NewCollector("suite.art")

	c.SetCurrentSuite("voltage-suite")

	c.RecordTestStart("measure-5v")
	c.RecordTestPass("measure-5v", "within tolerance")

	c.RecordTestStart("measure-12v")
	c.RecordTestFail("measure-12v", "out of range")

	c.ClearCurrentSuite()

	report := c.Finalize()

	if len(report.Suites) != 1 {
		t.Fatalf("expected 1 suite, got %d", len(report.Suites))
	}
	suite := report.Suites[0]
	if suite.Name != "voltage-suite" {
		t.Errorf("suite name = %q, want %q", suite.Name, "voltage-suite")
	}
	if len(suite.Tests) != 2 {
		t.Fatalf("expected 2 tests in suite, got %d", len(suite.Tests))
	}
	if suite.Summary.Total != 2 {
		t.Errorf("suite Total = %d, want 2", suite.Summary.Total)
	}
	if suite.Summary.Passed != 1 {
		t.Errorf("suite Passed = %d, want 1", suite.Summary.Passed)
	}
	if suite.Summary.Failed != 1 {
		t.Errorf("suite Failed = %d, want 1", suite.Summary.Failed)
	}
}

func TestMultipleSuites(t *testing.T) {
	c := NewCollector("multi-suite.art")

	c.SetCurrentSuite("suite-a")
	c.RecordTestStart("a1")
	c.RecordTestPass("a1", "")
	c.ClearCurrentSuite()

	c.SetCurrentSuite("suite-b")
	c.RecordTestStart("b1")
	c.RecordTestFail("b1", "bad")
	c.RecordTestStart("b2")
	c.RecordTestSkip("b2", "skip")
	c.ClearCurrentSuite()

	report := c.Finalize()

	if len(report.Suites) != 2 {
		t.Fatalf("expected 2 suites, got %d", len(report.Suites))
	}
	if report.Suites[0].Summary.Passed != 1 {
		t.Errorf("suite-a Passed = %d, want 1", report.Suites[0].Summary.Passed)
	}
	if report.Suites[1].Summary.Failed != 1 {
		t.Errorf("suite-b Failed = %d, want 1", report.Suites[1].Summary.Failed)
	}
	if report.Suites[1].Summary.Skipped != 1 {
		t.Errorf("suite-b Skipped = %d, want 1", report.Suites[1].Summary.Skipped)
	}
	// Overall summary: 3 tests total (1 passed, 1 failed, 1 skipped).
	if report.Summary.Total != 3 {
		t.Errorf("overall Total = %d, want 3", report.Summary.Total)
	}
}

// ---------------------------------------------------------------------------
// Standalone + suite tests mixed
// ---------------------------------------------------------------------------

func TestStandaloneAndSuiteTests(t *testing.T) {
	c := NewCollector("mixed.art")

	// Standalone test first.
	c.RecordTestStart("standalone-1")
	c.RecordTestPass("standalone-1", "ok")

	// Suite.
	c.SetCurrentSuite("my-suite")
	c.RecordTestStart("suite-test-1")
	c.RecordTestFail("suite-test-1", "nope")
	c.ClearCurrentSuite()

	// Another standalone.
	c.RecordTestStart("standalone-2")
	c.RecordTestPass("standalone-2", "ok")

	report := c.Finalize()

	if len(report.Tests) != 2 {
		t.Errorf("standalone tests = %d, want 2", len(report.Tests))
	}
	if len(report.Suites) != 1 {
		t.Errorf("suites = %d, want 1", len(report.Suites))
	}
	if report.Summary.Total != 3 {
		t.Errorf("Total = %d, want 3", report.Summary.Total)
	}
	if report.Summary.Passed != 2 {
		t.Errorf("Passed = %d, want 2", report.Summary.Passed)
	}
	if report.Summary.Failed != 1 {
		t.Errorf("Failed = %d, want 1", report.Summary.Failed)
	}
}

// ---------------------------------------------------------------------------
// Runtime errors
// ---------------------------------------------------------------------------

func TestRuntimeError(t *testing.T) {
	c := NewCollector("err.art")
	c.RecordError("undefined variable: x")
	c.RecordError("division by zero")
	report := c.Finalize()

	if len(report.Errors) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(report.Errors))
	}
	if report.Errors[0] != "undefined variable: x" {
		t.Errorf("error[0] = %q, want %q", report.Errors[0], "undefined variable: x")
	}
	if report.Errors[1] != "division by zero" {
		t.Errorf("error[1] = %q, want %q", report.Errors[1], "division by zero")
	}
}

// ---------------------------------------------------------------------------
// Finalize timing
// ---------------------------------------------------------------------------

func TestFinalizeComputesDuration(t *testing.T) {
	c := NewCollector("dur.art")
	c.RecordTestStart("t1")
	c.RecordTestPass("t1", "ok")
	report := c.Finalize()

	if report.StartTime.IsZero() {
		t.Error("StartTime should not be zero")
	}
	if report.EndTime.IsZero() {
		t.Error("EndTime should not be zero")
	}
	if report.EndTime.Before(report.StartTime) {
		t.Error("EndTime should not be before StartTime")
	}
	if report.Duration < 0 {
		t.Errorf("Duration = %v, should be >= 0", report.Duration)
	}
}

func TestFinalizeSummaryAggregatesAll(t *testing.T) {
	c := NewCollector("agg.art")

	// Suite with 1 pass, 1 fail.
	c.SetCurrentSuite("s1")
	c.RecordTestStart("s1-pass")
	c.RecordTestPass("s1-pass", "")
	c.RecordTestStart("s1-fail")
	c.RecordTestFail("s1-fail", "")
	c.ClearCurrentSuite()

	// Standalone: 1 skip, 1 error.
	c.RecordTestStart("standalone-skip")
	c.RecordTestSkip("standalone-skip", "")
	c.RecordTestStart("standalone-error")
	c.RecordTestError("standalone-error", "boom")

	report := c.Finalize()

	if report.Summary.Total != 4 {
		t.Errorf("Total = %d, want 4", report.Summary.Total)
	}
	if report.Summary.Passed != 1 {
		t.Errorf("Passed = %d, want 1", report.Summary.Passed)
	}
	if report.Summary.Failed != 1 {
		t.Errorf("Failed = %d, want 1", report.Summary.Failed)
	}
	if report.Summary.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", report.Summary.Skipped)
	}
	if report.Summary.Errors != 1 {
		t.Errorf("Errors = %d, want 1", report.Summary.Errors)
	}
}

// ---------------------------------------------------------------------------
// Message fields
// ---------------------------------------------------------------------------

func TestTestPassMessage(t *testing.T) {
	c := NewCollector("msg.art")
	c.RecordTestStart("t1")
	c.RecordTestPass("t1", "voltage within 5% tolerance")
	report := c.Finalize()

	if report.Tests[0].Message != "voltage within 5% tolerance" {
		t.Errorf("Message = %q, want %q", report.Tests[0].Message, "voltage within 5% tolerance")
	}
}

// ---------------------------------------------------------------------------
// Empty report
// ---------------------------------------------------------------------------

func TestEmptyReport(t *testing.T) {
	c := NewCollector("empty.art")
	report := c.Finalize()

	if report.Summary.Total != 0 {
		t.Errorf("Total = %d, want 0", report.Summary.Total)
	}
	if report.Summary.Passed != 0 {
		t.Errorf("Passed = %d, want 0", report.Summary.Passed)
	}
	if report.Summary.Failed != 0 {
		t.Errorf("Failed = %d, want 0", report.Summary.Failed)
	}
	if report.Summary.Skipped != 0 {
		t.Errorf("Skipped = %d, want 0", report.Summary.Skipped)
	}
	if report.Summary.Errors != 0 {
		t.Errorf("Errors = %d, want 0", report.Summary.Errors)
	}
	if report.ScriptPath != "empty.art" {
		t.Errorf("ScriptPath = %q, want %q", report.ScriptPath, "empty.art")
	}
}

// ---------------------------------------------------------------------------
// Multiple assertions in a test
// ---------------------------------------------------------------------------

func TestMultipleAssertions(t *testing.T) {
	c := NewCollector("multi-assert.art")
	c.RecordTestStart("t1")
	c.RecordAssertion("t1", true, "voltage > 4.5")
	c.RecordAssertion("t1", true, "voltage < 5.5")
	c.RecordAssertion("t1", false, "current == 0.5")
	c.RecordTestFail("t1", "assertion failed")
	report := c.Finalize()

	assertions := report.Tests[0].Assertions
	if len(assertions) != 3 {
		t.Fatalf("expected 3 assertions, got %d", len(assertions))
	}

	passCount := 0
	failCount := 0
	for _, a := range assertions {
		if a.Passed {
			passCount++
		} else {
			failCount++
		}
	}
	if passCount != 2 {
		t.Errorf("passed assertions = %d, want 2", passCount)
	}
	if failCount != 1 {
		t.Errorf("failed assertions = %d, want 1", failCount)
	}
}

// ---------------------------------------------------------------------------
// Multiple commands in a test
// ---------------------------------------------------------------------------

func TestMultipleCommands(t *testing.T) {
	c := NewCollector("multi-cmd.art")
	c.RecordTestStart("t1")
	c.RecordCommand("t1", "dmm-1", "*RST", true, "", 10)
	c.RecordCommand("t1", "dmm-1", "MEAS:VOLT?", true, "5.023", 85)
	c.RecordCommand("t1", "psu-1", "OUT ON", true, "OK", 5)
	c.RecordTestPass("t1", "done")
	report := c.Finalize()

	cmds := report.Tests[0].Commands
	if len(cmds) != 3 {
		t.Fatalf("expected 3 commands, got %d", len(cmds))
	}
	if cmds[0].CommandName != "*RST" {
		t.Errorf("cmd[0].CommandName = %q, want %q", cmds[0].CommandName, "*RST")
	}
	if cmds[1].Response != "5.023" {
		t.Errorf("cmd[1].Response = %q, want %q", cmds[1].Response, "5.023")
	}
	if cmds[2].DeviceID != "psu-1" {
		t.Errorf("cmd[2].DeviceID = %q, want %q", cmds[2].DeviceID, "psu-1")
	}
}

// ---------------------------------------------------------------------------
// Suite summary counts
// ---------------------------------------------------------------------------

func TestSuiteSummaryCounts(t *testing.T) {
	c := NewCollector("suite-counts.art")

	c.SetCurrentSuite("s1")
	c.RecordTestStart("p1")
	c.RecordTestPass("p1", "")
	c.RecordTestStart("p2")
	c.RecordTestPass("p2", "")
	c.RecordTestStart("f1")
	c.RecordTestFail("f1", "")
	c.RecordTestStart("sk1")
	c.RecordTestSkip("sk1", "")
	c.RecordTestStart("e1")
	c.RecordTestError("e1", "")
	c.ClearCurrentSuite()

	report := c.Finalize()

	suite := report.Suites[0]
	if suite.Summary.Total != 5 {
		t.Errorf("suite Total = %d, want 5", suite.Summary.Total)
	}
	if suite.Summary.Passed != 2 {
		t.Errorf("suite Passed = %d, want 2", suite.Summary.Passed)
	}
	if suite.Summary.Failed != 1 {
		t.Errorf("suite Failed = %d, want 1", suite.Summary.Failed)
	}
	if suite.Summary.Skipped != 1 {
		t.Errorf("suite Skipped = %d, want 1", suite.Summary.Skipped)
	}
	if suite.Summary.Errors != 1 {
		t.Errorf("suite Errors = %d, want 1", suite.Summary.Errors)
	}
}

// ---------------------------------------------------------------------------
// Graceful handling: RecordTestPass without RecordTestStart
// ---------------------------------------------------------------------------

func TestRecordTestPassWithoutStart(t *testing.T) {
	// Must not panic.
	c := NewCollector("no-start.art")
	c.RecordTestPass("orphan", "created on the fly")
	report := c.Finalize()

	if len(report.Tests) != 1 {
		t.Fatalf("expected 1 test, got %d", len(report.Tests))
	}
	if report.Tests[0].Name != "orphan" {
		t.Errorf("Name = %q, want %q", report.Tests[0].Name, "orphan")
	}
	if report.Tests[0].Status != "passed" {
		t.Errorf("Status = %q, want %q", report.Tests[0].Status, "passed")
	}
	if report.Summary.Total != 1 || report.Summary.Passed != 1 {
		t.Errorf("summary = %+v, want total=1 passed=1", report.Summary)
	}
}
