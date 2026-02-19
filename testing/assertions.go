package testing

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// TestAssertion represents a test assertion result
type TestAssertion struct {
	Name    string
	Passed  bool
	Message string
	Error   error
}

// AssertionResult tracks multiple assertions
type AssertionResult struct {
	Total   int
	Passed  int
	Failed  int
	Results []TestAssertion
}

// AssertEqual asserts equality
func AssertEqual(t *testing.T, expected, actual interface{}, message string) bool {
	if expected != actual {
		t.Errorf("%s: expected %v, got %v", message, expected, actual)
		return false
	}
	return true
}

// AssertNotEqual asserts inequality
func AssertNotEqual(t *testing.T, notExpected, actual interface{}, message string) bool {
	if notExpected == actual {
		t.Errorf("%s: expected not %v, got %v", message, notExpected, actual)
		return false
	}
	return true
}

// AssertNil asserts that value is nil
func AssertNil(t *testing.T, value interface{}, message string) bool {
	if value != nil {
		t.Errorf("%s: expected nil, got %v", message, value)
		return false
	}
	return true
}

// AssertNotNil asserts that value is not nil
func AssertNotNil(t *testing.T, value interface{}, message string) bool {
	if value == nil {
		t.Errorf("%s: expected not nil", message)
		return false
	}
	return true
}

// AssertTrue asserts that condition is true
func AssertTrue(t *testing.T, condition bool, message string) bool {
	if !condition {
		t.Errorf("%s: expected true, got false", message)
		return false
	}
	return true
}

// AssertFalse asserts that condition is false
func AssertFalse(t *testing.T, condition bool, message string) bool {
	if condition {
		t.Errorf("%s: expected false, got true", message)
		return false
	}
	return true
}

// AssertError asserts that an error occurred
func AssertError(t *testing.T, err error, message string) bool {
	if err == nil {
		t.Errorf("%s: expected error but got none", message)
		return false
	}
	return true
}

// AssertNoError asserts that no error occurred
func AssertNoError(t *testing.T, err error, message string) bool {
	if err != nil {
		t.Errorf("%s: unexpected error: %v", message, err)
		return false
	}
	return true
}

// AssertContains asserts that container contains item
func AssertContains(t *testing.T, container interface{}, item interface{}, message string) bool {
	switch c := container.(type) {
	case []string:
		for _, v := range c {
			if v == item {
				return true
			}
		}
	case []int:
		for _, v := range c {
			if v == item {
				return true
			}
		}
	case map[string]interface{}:
		if _, ok := c[item.(string)]; ok {
			return true
		}
	}

	t.Errorf("%s: container does not contain item %v", message, item)
	return false
}

// AssertLength asserts that collection has expected length
func AssertLength(t *testing.T, collection interface{}, expectedLen int, message string) bool {
	var actualLen int

	switch c := collection.(type) {
	case []interface{}:
		actualLen = len(c)
	case []string:
		actualLen = len(c)
	case []int:
		actualLen = len(c)
	case map[string]interface{}:
		actualLen = len(c)
	case string:
		actualLen = len(c)
	default:
		t.Errorf("%s: unsupported collection type", message)
		return false
	}

	if actualLen != expectedLen {
		t.Errorf("%s: expected length %d, got %d", message, expectedLen, actualLen)
		return false
	}
	return true
}

// AssertCompletesWithin asserts that operation completes within timeout
func AssertCompletesWithin(t *testing.T, duration time.Duration, message string, fn func() error) bool {
	done := make(chan error, 1)
	go func() {
		done <- fn()
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("%s: operation failed: %v", message, err)
			return false
		}
		return true
	case <-time.After(duration):
		t.Errorf("%s: operation exceeded timeout %v", message, duration)
		return false
	}
}

// AssertJSONValid asserts that data is valid JSON
func AssertJSONValid(t *testing.T, data []byte, message string) bool {
	var obj interface{}
	if err := json.Unmarshal(data, &obj); err != nil {
		t.Errorf("%s: invalid JSON: %v", message, err)
		return false
	}
	return true
}

// AssertJSONEqual asserts that two JSON objects are equal
func AssertJSONEqual(t *testing.T, expected, actual []byte, message string) bool {
	var exp, act interface{}

	if err := json.Unmarshal(expected, &exp); err != nil {
		t.Errorf("%s: failed to unmarshal expected: %v", message, err)
		return false
	}

	if err := json.Unmarshal(actual, &act); err != nil {
		t.Errorf("%s: failed to unmarshal actual: %v", message, err)
		return false
	}

	expJSON, _ := json.Marshal(exp)
	actJSON, _ := json.Marshal(act)

	if string(expJSON) != string(actJSON) {
		t.Errorf("%s: expected %s, got %s", message, string(expJSON), string(actJSON))
		return false
	}
	return true
}

// AssertStatusCode asserts HTTP status code
func AssertStatusCode(t *testing.T, expected, actual int, message string) bool {
	if expected != actual {
		t.Errorf("%s: expected status %d, got %d", message, expected, actual)
		return false
	}
	return true
}

// AssertStringContains asserts that string contains substring
func AssertStringContains(t *testing.T, str, substr, message string) bool {
	if !stringContains(str, substr) {
		t.Errorf("%s: string does not contain substring %q", message, substr)
		return false
	}
	return true
}

// AssertRange asserts that value is within range
func AssertRange(t *testing.T, value, min, max interface{}, message string) bool {
	valFloat := toFloat(value)
	minFloat := toFloat(min)
	maxFloat := toFloat(max)

	if valFloat < minFloat || valFloat > maxFloat {
		t.Errorf("%s: expected value between %v and %v, got %v", message, min, max, value)
		return false
	}
	return true
}

// Helper functions

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func toFloat(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case int32:
		return float64(val)
	default:
		return 0
	}
}

// TestResult represents the result of a test
type TestResult struct {
	Name      string
	Passed    bool
	Duration  time.Duration
	Error     string
	Logs      []string
}

// TestSuite groups related tests
type TestSuite struct {
	Name    string
	Results []TestResult
}

// RecordTestResult records a test result
func (ts *TestSuite) RecordTestResult(name string, passed bool, duration time.Duration, err error) {
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}

	result := TestResult{
		Name:     name,
		Passed:   passed,
		Duration: duration,
		Error:    errMsg,
	}

	ts.Results = append(ts.Results, result)
}

// Summary returns a summary of test results
func (ts *TestSuite) Summary() string {
	total := len(ts.Results)
	passed := 0

	for _, result := range ts.Results {
		if result.Passed {
			passed++
		}
	}

	failed := total - passed

	return fmt.Sprintf(
		"Test Suite: %s\nTotal: %d | Passed: %d | Failed: %d | Success Rate: %.1f%%",
		ts.Name,
		total,
		passed,
		failed,
		float64(passed)/float64(total)*100,
	)
}
