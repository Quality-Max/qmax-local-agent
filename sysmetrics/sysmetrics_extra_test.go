package sysmetrics

import (
	"encoding/json"
	"testing"
)

func TestMetrics_JSONRoundtrip(t *testing.T) {
	original := &Metrics{
		CPUPercent:    55.5,
		MemoryPercent: 72.3,
		ActiveTests:   5,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var parsed Metrics
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if parsed.CPUPercent != original.CPUPercent {
		t.Errorf("CPUPercent: got %f, want %f", parsed.CPUPercent, original.CPUPercent)
	}
	if parsed.MemoryPercent != original.MemoryPercent {
		t.Errorf("MemoryPercent: got %f, want %f", parsed.MemoryPercent, original.MemoryPercent)
	}
	if parsed.ActiveTests != original.ActiveTests {
		t.Errorf("ActiveTests: got %d, want %d", parsed.ActiveTests, original.ActiveTests)
	}
}

func TestMetrics_JSONFieldNames(t *testing.T) {
	m := &Metrics{CPUPercent: 10, MemoryPercent: 20, ActiveTests: 1}
	data, _ := json.Marshal(m)

	var raw map[string]interface{}
	_ = json.Unmarshal(data, &raw)

	// Verify JSON field names match the struct tags
	if _, ok := raw["cpu_percent"]; !ok {
		t.Error("expected 'cpu_percent' field in JSON")
	}
	if _, ok := raw["memory_percent"]; !ok {
		t.Error("expected 'memory_percent' field in JSON")
	}
	if _, ok := raw["active_tests"]; !ok {
		t.Error("expected 'active_tests' field in JSON")
	}
}

func TestParseVMStatLine_VariousFormats(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		prefix  string
		wantVal float64
		wantOK  bool
	}{
		{
			name:    "large number",
			line:    "Pages free:                              9999999.",
			prefix:  "Pages free",
			wantVal: 9999999,
			wantOK:  true,
		},
		{
			name:    "with extra whitespace",
			line:    "Pages active:                               42.",
			prefix:  "Pages active",
			wantVal: 42,
			wantOK:  true,
		},
		{
			name:    "multiple colons in line",
			line:    "Pages free: extra: 100.",
			prefix:  "Pages free",
			wantVal: 0,
			wantOK:  false, // split on ":" gives more than 2 parts
		},
		{
			name:    "prefix substring match",
			line:    "Pages speculative:                       500.",
			prefix:  "Pages speculative",
			wantVal: 500,
			wantOK:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, ok := parseVMStatLine(tt.line, tt.prefix)
			if ok != tt.wantOK {
				t.Errorf("ok: got %v, want %v", ok, tt.wantOK)
			}
			if ok && val != tt.wantVal {
				t.Errorf("val: got %f, want %f", val, tt.wantVal)
			}
		})
	}
}

func TestCollect_NegativeActiveTests(t *testing.T) {
	// Edge case: negative active tests count
	metrics := Collect(-1)
	if metrics != nil {
		if metrics.ActiveTests != -1 {
			t.Errorf("ActiveTests: got %d, want -1", metrics.ActiveTests)
		}
	}
}

func TestCollect_LargeActiveTests(t *testing.T) {
	metrics := Collect(100)
	if metrics != nil {
		if metrics.ActiveTests != 100 {
			t.Errorf("ActiveTests: got %d, want 100", metrics.ActiveTests)
		}
	}
}

// Test getCPUPercent directly
func TestGetCPUPercent_Callable(t *testing.T) {
	result := getCPUPercent()
	// We can't predict the exact value, just verify it doesn't panic
	_ = result
}

// Test getMemoryPercent directly
func TestGetMemoryPercent_Callable(t *testing.T) {
	result := getMemoryPercent()
	// We can't predict the exact value, just verify it doesn't panic
	_ = result
}
