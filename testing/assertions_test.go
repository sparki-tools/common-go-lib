package testing

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// TestAssertEqual tests equality assertion
func TestAssertEqual(t *testing.T) {
	if !AssertEqual(t, 5, 5, "values should be equal") {
		t.Error("assertion should pass for equal values")
	}
}

// TestAssertNotEqual tests inequality assertion
func TestAssertNotEqual(t *testing.T) {
	if !AssertNotEqual(t, 5, 6, "values should not be equal") {
		t.Error("assertion should pass for unequal values")
	}
}

// TestAssertNil tests nil assertion
func TestAssertNil(t *testing.T) {
	var value interface{}
	if !AssertNil(t, value, "value should be nil") {
		t.Error("assertion should pass for nil value")
	}
}

// TestAssertNotNil tests not nil assertion
func TestAssertNotNil(t *testing.T) {
	value := "test"
	if !AssertNotNil(t, value, "value should not be nil") {
		t.Error("assertion should pass for non-nil value")
	}
}

// TestAssertTrue tests true assertion
func TestAssertTrue(t *testing.T) {
	if !AssertTrue(t, true, "condition should be true") {
		t.Error("assertion should pass for true condition")
	}
}

// TestAssertFalse tests false assertion
func TestAssertFalse(t *testing.T) {
	if !AssertFalse(t, false, "condition should be false") {
		t.Error("assertion should pass for false condition")
	}
}

// TestAssertError tests error assertion
func TestAssertError(t *testing.T) {
	err := errors.New("test error")
	if !AssertError(t, err, "error should be present") {
		t.Error("assertion should pass when error is present")
	}
}

// TestAssertNoError tests no error assertion
func TestAssertNoError(t *testing.T) {
	if !AssertNoError(t, nil, "no error expected") {
		t.Error("assertion should pass when error is nil")
	}
}

// TestAssertContains tests contains assertion
func TestAssertContains(t *testing.T) {
	items := []string{"apple", "banana", "cherry"}
	if !AssertContains(t, items, "banana", "should contain banana") {
		t.Error("assertion should pass for contained item")
	}
}

// TestAssertLength tests length assertion
func TestAssertLength(t *testing.T) {
	items := []string{"a", "b", "c"}
	if !AssertLength(t, items, 3, "length should be 3") {
		t.Error("assertion should pass for correct length")
	}
}

// TestAssertLengthString tests length assertion for strings
func TestAssertLengthString(t *testing.T) {
	str := "hello"
	if !AssertLength(t, str, 5, "string length should be 5") {
		t.Error("assertion should pass for correct string length")
	}
}

// TestAssertCompletesWithin tests timeout assertion
func TestAssertCompletesWithin(t *testing.T) {
	passed := AssertCompletesWithin(t, 100*time.Millisecond, "operation should complete quickly", func() error {
		time.Sleep(10 * time.Millisecond)
		return nil
	})

	if !passed {
		t.Error("assertion should pass when operation completes within timeout")
	}
}

// TestAssertJSONValid tests JSON validation assertion
func TestAssertJSONValid(t *testing.T) {
	data := []byte(`{"name":"test","age":30}`)
	if !AssertJSONValid(t, data, "JSON should be valid") {
		t.Error("assertion should pass for valid JSON")
	}
}

// TestAssertJSONEqual tests JSON equality assertion
func TestAssertJSONEqual(t *testing.T) {
	obj1 := map[string]interface{}{"name": "test", "age": 30}
	obj2 := map[string]interface{}{"age": 30, "name": "test"}

	data1, _ := json.Marshal(obj1)
	data2, _ := json.Marshal(obj2)

	if !AssertJSONEqual(t, data1, data2, "JSON objects should be equal") {
		t.Error("assertion should pass for equal JSON objects")
	}
}

// TestAssertStatusCode tests status code assertion
func TestAssertStatusCode(t *testing.T) {
	if !AssertStatusCode(t, 200, 200, "status codes should match") {
		t.Error("assertion should pass for matching status codes")
	}
}

// TestAssertStringContains tests string contains assertion
func TestAssertStringContains(t *testing.T) {
	if !AssertStringContains(t, "hello world", "world", "string should contain substring") {
		t.Error("assertion should pass when string contains substring")
	}
}

// TestAssertRange tests range assertion
func TestAssertRange(t *testing.T) {
	if !AssertRange(t, 5, 1, 10, "value should be in range") {
		t.Error("assertion should pass for value in range")
	}
}

// TestTestSuiteRecordResult tests recording test results
func TestTestSuiteRecordResult(t *testing.T) {
	suite := &TestSuite{Name: "TestSuite1"}

	suite.RecordTestResult("test1", true, 100*time.Millisecond, nil)
	suite.RecordTestResult("test2", false, 50*time.Millisecond, errors.New("failed"))

	if len(suite.Results) != 2 {
		t.Errorf("expected 2 results, got %d", len(suite.Results))
	}

	if suite.Results[0].Name != "test1" {
		t.Error("first result name incorrect")
	}

	if !suite.Results[0].Passed {
		t.Error("first result should be passed")
	}

	if suite.Results[1].Passed {
		t.Error("second result should be failed")
	}
}

// TestTestSuiteSummary tests test suite summary
func TestTestSuiteSummary(t *testing.T) {
	suite := &TestSuite{Name: "TestSuite1"}

	suite.RecordTestResult("test1", true, 100*time.Millisecond, nil)
	suite.RecordTestResult("test2", true, 100*time.Millisecond, nil)
	suite.RecordTestResult("test3", false, 100*time.Millisecond, errors.New("failed"))

	summary := suite.Summary()
	if summary == "" {
		t.Error("summary should not be empty")
	}

	if !contains(summary, "TestSuite1") {
		t.Error("summary should contain suite name")
	}

	if !contains(summary, "Total: 3") {
		t.Error("summary should contain total count")
	}
}

// TestAssertEqualMultipleTypes tests equality with different types
func TestAssertEqualMultipleTypes(t *testing.T) {
	// String equality
	AssertEqual(t, "hello", "hello", "strings should be equal")

	// Integer equality
	AssertEqual(t, 42, 42, "integers should be equal")

	// Boolean equality
	AssertEqual(t, true, true, "booleans should be equal")
}

// TestAssertLengthMultipleTypes tests length assertion with different types
func TestAssertLengthMultipleTypes(t *testing.T) {
	// Slice
	AssertLength(t, []int{1, 2, 3}, 3, "slice length should be 3")

	// String
	AssertLength(t, "test", 4, "string length should be 4")

	// Map
	m := map[string]interface{}{"a": 1, "b": 2}
	AssertLength(t, m, 2, "map length should be 2")
}

// TestAssertRangeMultipleTypes tests range assertion with different types
func TestAssertRangeMultipleTypes(t *testing.T) {
	// Float
	AssertRange(t, 5.5, 1.0, 10.0, "float should be in range")

	// Int32
	AssertRange(t, int32(5), int32(1), int32(10), "int32 should be in range")

	// Int64
	AssertRange(t, int64(5), int64(1), int64(10), "int64 should be in range")
}

// Helper function
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
