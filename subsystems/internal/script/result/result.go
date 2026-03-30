// Package result collects test results during script execution and produces
// structured reports. The executor depends on this package via an interface;
// this package does NOT import the executor.
package result

import "time"

// ---------------------------------------------------------------------------
// Data types
// ---------------------------------------------------------------------------

// CommandResult holds the outcome of a device command.
type CommandResult struct {
	DeviceID    string `json:"device_id"`
	CommandName string `json:"command_name"`
	Response    string `json:"response"`
	Success     bool   `json:"success"`
	DurationMs  int    `json:"duration_ms"`
}

// Assertion holds the outcome of a single ASSERT statement.
type Assertion struct {
	Passed  bool   `json:"passed"`
	Message string `json:"message"`
}

// TestResult holds the outcome of a single TEST block.
type TestResult struct {
	Name       string          `json:"name"`
	Status     string          `json:"status"` // "passed", "failed", "skipped", "error"
	Message    string          `json:"message,omitempty"`
	Assertions []Assertion     `json:"assertions,omitempty"`
	Commands   []CommandResult `json:"commands,omitempty"`
	StartTime  time.Time       `json:"start_time"`
	EndTime    time.Time       `json:"end_time"`
	Duration   time.Duration   `json:"duration"`
}

// SuiteResult holds the outcome of a SUITE block.
type SuiteResult struct {
	Name    string        `json:"name"`
	Tests   []*TestResult `json:"tests"`
	Summary Summary       `json:"summary"`
}

// Summary aggregates pass/fail/skip counts.
type Summary struct {
	Total   int `json:"total"`
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
	Errors  int `json:"errors"`
}

// RunReport is the top-level report for a full script execution.
type RunReport struct {
	ScriptPath string         `json:"script_path"`
	Suites     []*SuiteResult `json:"suites,omitempty"`
	Tests      []*TestResult  `json:"tests,omitempty"` // standalone tests (not in suites)
	Summary    Summary        `json:"summary"`
	Errors     []string       `json:"errors,omitempty"` // runtime errors
	StartTime  time.Time      `json:"start_time"`
	EndTime    time.Time      `json:"end_time"`
	Duration   time.Duration  `json:"duration"`
}

// ---------------------------------------------------------------------------
// Collector
// ---------------------------------------------------------------------------

// Collector accumulates test results during script execution.
type Collector struct {
	scriptPath string
	suites     []*SuiteResult
	tests      []*TestResult // standalone tests
	errors     []string
	startTime  time.Time

	// Current state
	currentTest  *TestResult
	currentSuite *SuiteResult
}

// NewCollector creates a Collector for the given script path, recording the
// start time immediately.
func NewCollector(scriptPath string) *Collector {
	return &Collector{
		scriptPath: scriptPath,
		startTime:  time.Now(),
	}
}

// ---------------------------------------------------------------------------
// Test lifecycle
// ---------------------------------------------------------------------------

// RecordTestStart creates a new TestResult with status "running".
func (c *Collector) RecordTestStart(name string) {
	c.currentTest = &TestResult{
		Name:      name,
		Status:    "running",
		StartTime: time.Now(),
	}
}

// RecordTestPass marks the current test as passed and finalises it.
// If no matching test is in progress, a synthetic result is created.
func (c *Collector) RecordTestPass(name, message string) {
	c.finishTest(name, "passed", message)
}

// RecordTestFail marks the current test as failed and finalises it.
// If no matching test is in progress, a synthetic result is created.
func (c *Collector) RecordTestFail(name, message string) {
	c.finishTest(name, "failed", message)
}

// RecordTestSkip marks the current test as skipped and finalises it.
// If no matching test is in progress, a synthetic result is created.
func (c *Collector) RecordTestSkip(name, message string) {
	c.finishTest(name, "skipped", message)
}

// RecordTestError marks the current test as errored and finalises it.
// If no matching test is in progress, a synthetic result is created.
func (c *Collector) RecordTestError(name, message string) {
	c.finishTest(name, "error", message)
}

// finishTest is the shared implementation for all test-finishing methods.
func (c *Collector) finishTest(name, status, message string) {
	tr := c.currentTest
	if tr == nil || tr.Name != name {
		// No matching current test -- create a synthetic one.
		tr = &TestResult{
			Name:      name,
			StartTime: time.Now(),
		}
	}

	now := time.Now()
	tr.Status = status
	tr.Message = message
	tr.EndTime = now
	tr.Duration = now.Sub(tr.StartTime)

	if c.currentSuite != nil {
		c.currentSuite.Tests = append(c.currentSuite.Tests, tr)
	} else {
		c.tests = append(c.tests, tr)
	}

	// Clear current test.
	c.currentTest = nil
}

// ---------------------------------------------------------------------------
// Events within tests
// ---------------------------------------------------------------------------

// RecordAssertion appends an assertion result to the current test.
// If there is no current test the call is ignored.
func (c *Collector) RecordAssertion(testName string, passed bool, message string) {
	if c.currentTest == nil {
		return
	}
	c.currentTest.Assertions = append(c.currentTest.Assertions, Assertion{
		Passed:  passed,
		Message: message,
	})
}

// RecordCommand appends a command result to the current test.
// If there is no current test the call is ignored.
func (c *Collector) RecordCommand(testName, deviceID, command string, success bool, response string, durationMs int) {
	if c.currentTest == nil {
		return
	}
	c.currentTest.Commands = append(c.currentTest.Commands, CommandResult{
		DeviceID:    deviceID,
		CommandName: command,
		Success:     success,
		Response:    response,
		DurationMs:  durationMs,
	})
}

// ---------------------------------------------------------------------------
// Suite lifecycle
// ---------------------------------------------------------------------------

// SetCurrentSuite creates a new SuiteResult and sets it as the current suite.
func (c *Collector) SetCurrentSuite(name string) {
	c.currentSuite = &SuiteResult{
		Name: name,
	}
}

// ClearCurrentSuite computes the suite summary, adds it to the suites list,
// and clears the current suite.
func (c *Collector) ClearCurrentSuite() {
	if c.currentSuite == nil {
		return
	}
	c.currentSuite.Summary = summarizeTests(c.currentSuite.Tests)
	c.suites = append(c.suites, c.currentSuite)
	c.currentSuite = nil
}

// ---------------------------------------------------------------------------
// Runtime errors
// ---------------------------------------------------------------------------

// RecordError records a runtime error that is not associated with a test.
func (c *Collector) RecordError(message string) {
	c.errors = append(c.errors, message)
}

// ---------------------------------------------------------------------------
// Finalize
// ---------------------------------------------------------------------------

// Finalize computes summaries and durations. Call after execution completes.
func (c *Collector) Finalize() *RunReport {
	now := time.Now()
	report := &RunReport{
		ScriptPath: c.scriptPath,
		Suites:     c.suites,
		Tests:      c.tests,
		Errors:     c.errors,
		StartTime:  c.startTime,
		EndTime:    now,
		Duration:   now.Sub(c.startTime),
	}

	// Aggregate summary across all suites and standalone tests.
	var allTests []*TestResult
	for _, s := range c.suites {
		allTests = append(allTests, s.Tests...)
	}
	allTests = append(allTests, c.tests...)
	report.Summary = summarizeTests(allTests)

	return report
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// summarizeTests computes a Summary from a slice of TestResult pointers.
func summarizeTests(tests []*TestResult) Summary {
	var s Summary
	s.Total = len(tests)
	for _, t := range tests {
		switch t.Status {
		case "passed":
			s.Passed++
		case "failed":
			s.Failed++
		case "skipped":
			s.Skipped++
		case "error":
			s.Errors++
		}
	}
	return s
}
