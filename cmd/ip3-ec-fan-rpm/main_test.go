package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecodeFanRPM(t *testing.T) {
	if got := decodeFanRPM(0x56, 0x06); got != 1622 {
		t.Fatalf("fan1 rpm = %d, want 1622", got)
	}
	if got := decodeFanRPM(0x00, 0x00); got != 0 {
		t.Fatalf("fan2 rpm = %d, want 0", got)
	}
}

func TestParseMilliCelsius(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want float64
		ok   bool
	}{
		{"valid", "42500\n", 42.5, true},
		{"empty", "", 0, false},
		{"invalid", "nope", 0, false},
		{"negative", "-1000", 0, false},
		{"zero", "0", 0, false},
		{"too high", "130000", 0, false},
	}

	for _, tt := range tests {
		got, ok := parseMilliCelsius(tt.in)
		if ok != tt.ok {
			t.Fatalf("%s ok = %v, want %v", tt.name, ok, tt.ok)
		}
		if got != tt.want {
			t.Fatalf("%s temp = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestCollectThermalFromDir(t *testing.T) {
	dir := t.TempDir()
	writeHwmon(t, dir, 0, "k10temp", map[string]string{
		"temp1_input": "42500\n",
		"temp2_input": "-1000\n",
	})
	writeHwmon(t, dir, 1, "nvme", map[string]string{
		"temp1_input": "76000\n",
		"temp2_input": "invalid\n",
	})

	thermal := collectThermalFromDir(dir)
	if !thermal.Readable {
		t.Fatal("thermal should be readable")
	}
	if thermal.SensorCount != 2 {
		t.Fatalf("sensor count = %d, want 2", thermal.SensorCount)
	}
	if thermal.MaxTempCelsius != 76.0 {
		t.Fatalf("max temp = %v, want 76.0", thermal.MaxTempCelsius)
	}
	if !thermal.ControlTempReadable {
		t.Fatal("control temp should be readable")
	}
	if thermal.ControlTempCelsius != 42.5 {
		t.Fatalf("control temp = %v, want 42.5", thermal.ControlTempCelsius)
	}
	if thermal.ControlTempSource != controlTempSourcePreferred {
		t.Fatalf("control source = %s, want preferred", thermal.ControlTempSource)
	}
}

func TestCollectThermalFallbackToMax(t *testing.T) {
	dir := t.TempDir()
	writeHwmon(t, dir, 0, "nvme", map[string]string{"temp1_input": "52000\n"})

	thermal := collectThermalFromDir(dir)
	if !thermal.ControlTempReadable {
		t.Fatal("control temp should fallback to max temp")
	}
	if thermal.ControlTempCelsius != 52.0 {
		t.Fatalf("control temp = %v, want 52.0", thermal.ControlTempCelsius)
	}
	if thermal.ControlTempSource != controlTempSourceFallbackMax {
		t.Fatalf("control source = %s, want fallback_max", thermal.ControlTempSource)
	}
}

func TestCollectThermalUnavailable(t *testing.T) {
	thermal := collectThermalFromDir(t.TempDir())
	if thermal.Readable || thermal.ControlTempReadable {
		t.Fatalf("thermal should be unreadable: %#v", thermal)
	}
	if thermal.ControlTempSource != controlTempSourceUnavailable {
		t.Fatalf("control source = %s, want unavailable", thermal.ControlTempSource)
	}
}

func TestEvaluateFanStatus(t *testing.T) {
	tests := []struct {
		name    string
		fan     FanData
		thermal ThermalData
		want    string
	}{
		{
			name: "unreadable",
			fan:  FanData{Readable: false, RPMAvailable: false},
			want: fanStateUnreadable,
		},
		{
			name: "normal",
			fan:  FanData{Readable: true, RPMAvailable: true, Fan1RPM: 1200},
			want: fanStateNormal,
		},
		{
			name:    "zero temp unknown",
			fan:     FanData{Readable: true, RPMAvailable: true, Fan1RPM: 0},
			thermal: ThermalData{Readable: false},
			want:    fanStateFanZeroTempUnknown,
		},
		{
			name:    "low temp stop",
			fan:     FanData{Readable: true, RPMAvailable: true, Fan1RPM: 0},
			thermal: ThermalData{Readable: true, MaxTempCelsius: 80.0, ControlTempReadable: true, ControlTempCelsius: 44.9},
			want:    fanStateFanStopLowTemp,
		},
		{
			name:    "suspicious zero",
			fan:     FanData{Readable: true, RPMAvailable: true, Fan1RPM: 0},
			thermal: ThermalData{Readable: true, ControlTempReadable: true, ControlTempCelsius: 45.0},
			want:    fanStateSuspiciousZeroRPM,
		},
		{
			name:    "critical zero",
			fan:     FanData{Readable: true, RPMAvailable: true, Fan1RPM: 0},
			thermal: ThermalData{Readable: true, ControlTempReadable: true, ControlTempCelsius: 65.0},
			want:    fanStateCriticalZeroRPMHighTemp,
		},
	}

	for _, tt := range tests {
		got := evaluateFanStatus(tt.fan, tt.thermal)
		if got.Fan1State != tt.want {
			t.Fatalf("%s state = %s, want %s", tt.name, got.Fan1State, tt.want)
		}
		if got.ReasonCode == "" || !strings.Contains(got.Reason, "instant sample only") {
			t.Fatalf("%s reason is not stable and explicit: %#v", tt.name, got)
		}
		if tt.want != fanStateNormal && tt.want != fanStateUnreadable && tt.want != fanStateFanZeroTempUnknown &&
			!strings.Contains(got.Reason, "control_temp_celsius") {
			t.Fatalf("%s reason should mention control temp: %#v", tt.name, got)
		}
	}
}

func TestPrometheusEscape(t *testing.T) {
	got := prometheusEscape("a\\b\"c\nd")
	want := `a\\b\"c\nd`
	if got != want {
		t.Fatalf("escaped = %q, want %q", got, want)
	}
}

func TestFanStatusOneHot(t *testing.T) {
	snapshot := Snapshot{
		DMI: DMIData{
			SysVendor:   strPtr(`ven"dor`),
			ProductName: strPtr("prod\\uct"),
		},
		Fan: FanData{RPMAvailable: true, Fan1RPM: 0, Fan2RPM: 0},
		Thermal: ThermalData{
			Readable:            true,
			SensorCount:         2,
			MaxTempCelsius:      66.2,
			ControlTempReadable: true,
			ControlTempCelsius:  42.5,
			ControlTempSource:   controlTempSourcePreferred,
		},
		Status: FanStatus{Fan1State: fanStateCriticalZeroRPMHighTemp},
	}

	prom := toPrometheus(snapshot)
	for _, state := range fanStatusStates() {
		want := 0
		if state == fanStateCriticalZeroRPMHighTemp {
			want = 1
		}
		line := fmt.Sprintf(`ip3_ec_fan_status{vendor="ven\"dor",product="prod\\uct",status="%s"} %d`, state, want)
		if !strings.Contains(prom, line) {
			t.Fatalf("missing one-hot line %q in:\n%s", line, prom)
		}
	}
	if !strings.Contains(prom, `ip3_ec_control_temp_celsius{vendor="ven\"dor",product="prod\\uct",source="preferred"} 42.5`) {
		t.Fatalf("missing control temp metric in:\n%s", prom)
	}
	if !strings.Contains(prom, `ip3_ec_control_temp_readable{vendor="ven\"dor",product="prod\\uct",source="preferred"} 1`) {
		t.Fatalf("missing control temp readable metric in:\n%s", prom)
	}
}

func strPtr(s string) *string {
	return &s
}

func writeHwmon(t *testing.T, root string, idx int, name string, files map[string]string) {
	t.Helper()
	dir := filepath.Join(root, fmt.Sprintf("hwmon%d", idx))
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "name"), []byte(name+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for file, value := range files {
		if err := os.WriteFile(filepath.Join(dir, file), []byte(value), 0644); err != nil {
			t.Fatal(err)
		}
	}
}
