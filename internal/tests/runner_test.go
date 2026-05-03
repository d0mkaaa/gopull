package tests

import (
	"testing"
	"time"
)

func TestRunSupportsHeaderJSONPathAndResponseTimeAssertions(t *testing.T) {
	script := `
assert header Content-Type contains json
assert jsonpath $.data.id >= 42
assert response_time < 500
`
	body := []byte(`{"data":{"id":42}}`)
	headers := "Content-Type: application/json\n"

	got := Run(script, 200, body, headers, 125*time.Millisecond)
	if len(got.Assertions) != 3 {
		t.Fatalf("got %d assertions, want 3", len(got.Assertions))
	}
	for _, a := range got.Assertions {
		if !a.Pass {
			t.Fatalf("%s failed with actual %q", a.Label, a.Actual)
		}
	}
}

func TestRunReportsJSONPathMiss(t *testing.T) {
	got := Run(`assert jsonpath $.missing == yes`, 200, []byte(`{"ok":true}`), "", 0)
	if len(got.Assertions) != 1 {
		t.Fatalf("got %d assertions, want 1", len(got.Assertions))
	}
	if got.Assertions[0].Pass {
		t.Fatal("missing JSONPath should fail")
	}
	if got.Assertions[0].Actual != "path not found" {
		t.Fatalf("actual: got %q", got.Assertions[0].Actual)
	}
}
