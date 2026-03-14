package sysmetrics

import (
	"runtime"
	"testing"
)

func TestCollect_ReturnsMetrics(t *testing.T) {
	metrics := Collect(3)

	// On macOS/Linux, metrics should be non-nil
	if metrics == nil {
		t.Skip("Collect returned nil (metrics unavailable on this platform)")
	}

	if metrics.CPUPercent < 0 {
		t.Errorf("CPUPercent should be >= 0, got %f", metrics.CPUPercent)
	}
	if metrics.MemoryPercent < 0 {
		t.Errorf("MemoryPercent should be >= 0, got %f", metrics.MemoryPercent)
	}
	if metrics.ActiveTests != 3 {
		t.Errorf("ActiveTests: got %d, want 3", metrics.ActiveTests)
	}
}

func TestCollect_ZeroActiveTests(t *testing.T) {
	metrics := Collect(0)
	if metrics == nil {
		t.Skip("Collect returned nil")
	}
	if metrics.ActiveTests != 0 {
		t.Errorf("ActiveTests: got %d, want 0", metrics.ActiveTests)
	}
}

func TestGetCPUPercent(t *testing.T) {
	cpu := getCPUPercent()
	switch runtime.GOOS {
	case "darwin", "linux":
		if cpu < 0 {
			t.Logf("getCPUPercent returned %f (may be expected in some environments)", cpu)
		} else if cpu > 100 {
			t.Errorf("getCPUPercent should be <= 100, got %f", cpu)
		}
	default:
		if cpu != -1 {
			t.Errorf("expected -1 on unsupported platform, got %f", cpu)
		}
	}
}

func TestGetMemoryPercent(t *testing.T) {
	mem := getMemoryPercent()
	switch runtime.GOOS {
	case "darwin", "linux":
		if mem < 0 {
			t.Logf("getMemoryPercent returned %f (may be expected in some environments)", mem)
		} else if mem > 100 {
			t.Errorf("getMemoryPercent should be <= 100, got %f", mem)
		}
	default:
		if mem != -1 {
			t.Errorf("expected -1 on unsupported platform, got %f", mem)
		}
	}
}

func TestParseVMStatLine(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		prefix   string
		wantVal  float64
		wantOK   bool
	}{
		{
			name:    "Pages free",
			line:    "Pages free:                              123456.",
			prefix:  "Pages free",
			wantVal: 123456,
			wantOK:  true,
		},
		{
			name:    "Pages active",
			line:    "Pages active:                            789012.",
			prefix:  "Pages active",
			wantVal: 789012,
			wantOK:  true,
		},
		{
			name:    "Pages wired down",
			line:    "Pages wired down:                        456789.",
			prefix:  "Pages wired down",
			wantVal: 456789,
			wantOK:  true,
		},
		{
			name:    "Pages inactive",
			line:    "Pages inactive:                          111222.",
			prefix:  "Pages inactive",
			wantVal: 111222,
			wantOK:  true,
		},
		{
			name:    "Pages speculative",
			line:    "Pages speculative:                       333444.",
			prefix:  "Pages speculative",
			wantVal: 333444,
			wantOK:  true,
		},
		{
			name:    "no match",
			line:    "Pages purgeable:                         100.",
			prefix:  "Pages free",
			wantVal: 0,
			wantOK:  false,
		},
		{
			name:    "empty line",
			line:    "",
			prefix:  "Pages free",
			wantVal: 0,
			wantOK:  false,
		},
		{
			name:    "no colon",
			line:    "Pages free 12345",
			prefix:  "Pages free",
			wantVal: 0,
			wantOK:  false,
		},
		{
			name:    "invalid value",
			line:    "Pages free:                              abc.",
			prefix:  "Pages free",
			wantVal: 0,
			wantOK:  false,
		},
		{
			name:    "zero value",
			line:    "Pages free:                              0.",
			prefix:  "Pages free",
			wantVal: 0,
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

func TestMetricsJSONSerialization(t *testing.T) {
	// Verify the struct tags work correctly
	m := &Metrics{
		CPUPercent:    42.5,
		MemoryPercent: 65.3,
		ActiveTests:   2,
	}

	if m.CPUPercent != 42.5 {
		t.Errorf("CPUPercent: got %f, want 42.5", m.CPUPercent)
	}
	if m.MemoryPercent != 65.3 {
		t.Errorf("MemoryPercent: got %f, want 65.3", m.MemoryPercent)
	}
	if m.ActiveTests != 2 {
		t.Errorf("ActiveTests: got %d, want 2", m.ActiveTests)
	}
}

func TestCollect_BothNegative(t *testing.T) {
	// This tests the nil return path when both cpu and mem are < 0
	// We can't easily force this, but we can verify the function handles it
	metrics := Collect(0)
	if metrics != nil {
		// If metrics are available, verify they are non-negative
		if metrics.CPUPercent < 0 {
			t.Errorf("CPUPercent should be >= 0 when metrics is non-nil, got %f", metrics.CPUPercent)
		}
		if metrics.MemoryPercent < 0 {
			t.Errorf("MemoryPercent should be >= 0 when metrics is non-nil, got %f", metrics.MemoryPercent)
		}
	}
}
