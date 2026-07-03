package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	ecIOPath    = "/sys/kernel/debug/ec/ec0/io"
	dmiDir      = "/sys/class/dmi/id"
	acpiDevices = "/sys/bus/acpi/devices"
	wmiDevices  = "/sys/bus/wmi/devices"
	hwmonDir    = "/sys/class/hwmon"

	fanStopTempC     = 45.0
	fanCriticalTempC = 65.0
)

const (
	fanStateUnreadable              = "unreadable"
	fanStateNormal                  = "normal"
	fanStateFanStopLowTemp          = "fan_stop_low_temp"
	fanStateSuspiciousZeroRPM       = "suspicious_zero_rpm"
	fanStateCriticalZeroRPMHighTemp = "critical_zero_rpm_high_temp"
	fanStateFanZeroTempUnknown      = "fan_zero_temp_unknown"
)

const (
	controlTempSourcePreferred   = "preferred"
	controlTempSourceFallbackMax = "fallback_max"
	controlTempSourceUnavailable = "unavailable"
)

type DMIData struct {
	SysVendor      *string `json:"sys_vendor"`
	ProductName    *string `json:"product_name"`
	ProductVersion *string `json:"product_version"`
	BoardVendor    *string `json:"board_vendor"`
	BoardName      *string `json:"board_name"`
	BiosVendor     *string `json:"bios_vendor"`
	BiosVersion    *string `json:"bios_version"`
	BiosDate       *string `json:"bios_date"`
}

type Signals struct {
	IP3PowerSwitch        bool `json:"ip3_power_switch"`
	IP3WMIEvent           bool `json:"ip3_wmi_event"`
	RWECRegWMI            bool `json:"rwec_reg_wmi"`
	WMIGUID99D89064       bool `json:"wmi_guid_99d89064"`
	WMIGUID8FAFC061       bool `json:"wmi_guid_8fafc061"`
	StandardHwmonFanAvail bool `json:"standard_hwmon_fan_available"`
	ECIOExists            bool `json:"ec_io_exists"`
}

type FanMode struct {
	FCMO0x31 int `json:"fcmo_0x31"`
	FCMI0x32 int `json:"fcmi_0x32"`
}

type FanControlReadOnly struct {
	Fan10x33 int `json:"fan1_0x33"`
	Fan20x34 int `json:"fan2_0x34"`
}

type FanRaw struct {
	EC0x35FN1L    int    `json:"ec_0x35_fn1l"`
	EC0x36FN1H    int    `json:"ec_0x36_fn1h"`
	EC0x37FN2L    int    `json:"ec_0x37_fn2l"`
	EC0x38FN2H    int    `json:"ec_0x38_fn2h"`
	EC0x310x38Hex string `json:"ec_0x31_0x38_hex"`
}

type FanFormula struct {
	Fan1 string `json:"fan1"`
	Fan2 string `json:"fan2"`
}

type FanData struct {
	Readable             bool                `json:"readable"`
	RPMAvailable         bool                `json:"rpm_available"`
	Error                *string             `json:"error,omitempty"`
	ECIOPath             string              `json:"ec_io_path"`
	Source               string              `json:"source,omitempty"`
	Fan1RPM              int                 `json:"fan1_rpm"`
	Fan2RPM              int                 `json:"fan2_rpm"`
	Fan1Available        bool                `json:"fan1_available"`
	Fan2Available        bool                `json:"fan2_available"`
	Mode                 *FanMode            `json:"mode,omitempty"`
	ControlBytesReadOnly *FanControlReadOnly `json:"control_bytes_read_only,omitempty"`
	Raw                  *FanRaw             `json:"raw,omitempty"`
	Formula              *FanFormula         `json:"formula,omitempty"`
}

type Safety struct {
	WritesEC               bool `json:"writes_ec"`
	RequiresECWriteSupport bool `json:"requires_ec_write_support"`
	ControlsFan            bool `json:"controls_fan"`
	ChangesPowerProfile    bool `json:"changes_power_profile"`
}

type ThermalData struct {
	Readable            bool    `json:"readable"`
	SensorCount         int     `json:"sensor_count"`
	MaxTempCelsius      float64 `json:"max_temp_celsius"`
	ControlTempReadable bool    `json:"control_temp_readable"`
	ControlTempCelsius  float64 `json:"control_temp_celsius"`
	ControlTempSource   string  `json:"control_temp_source"`
}

type FanStatus struct {
	Fan1State  string `json:"fan1_state"`
	ReasonCode string `json:"reason_code"`
	Reason     string `json:"reason"`
}

type Snapshot struct {
	Tool    string      `json:"tool"`
	Version string      `json:"version"`
	Mode    string      `json:"mode"`
	Safety  Safety      `json:"safety"`
	DMI     DMIData     `json:"dmi"`
	Signals Signals     `json:"signals"`
	Fan     FanData     `json:"fan"`
	Thermal ThermalData `json:"thermal"`
	Status  FanStatus   `json:"status"`
}

func readText(path string) *string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	s := strings.TrimSpace(string(data))
	return &s
}

func readDMI() DMIData {
	return DMIData{
		SysVendor:      readText(filepath.Join(dmiDir, "sys_vendor")),
		ProductName:    readText(filepath.Join(dmiDir, "product_name")),
		ProductVersion: readText(filepath.Join(dmiDir, "product_version")),
		BoardVendor:    readText(filepath.Join(dmiDir, "board_vendor")),
		BoardName:      readText(filepath.Join(dmiDir, "board_name")),
		BiosVendor:     readText(filepath.Join(dmiDir, "bios_vendor")),
		BiosVersion:    readText(filepath.Join(dmiDir, "bios_version")),
		BiosDate:       readText(filepath.Join(dmiDir, "bios_date")),
	}
}

func hasACPIUID(value string) bool {
	matches, err := filepath.Glob(filepath.Join(acpiDevices, "PNP0C14:*/uid"))
	if err != nil {
		return false
	}
	for _, uidFile := range matches {
		uid := readText(uidFile)
		if uid != nil && *uid == value {
			return true
		}
	}
	return false
}

func hasWMIGUID(guid string) bool {
	entries, err := os.ReadDir(wmiDevices)
	if err != nil {
		return false
	}
	target := strings.ToUpper(guid)
	for _, entry := range entries {
		if strings.ToUpper(entry.Name()) == target {
			return true
		}
	}
	return false
}

func standardHwmonFanAvailable() bool {
	matches, err := filepath.Glob(filepath.Join(hwmonDir, "hwmon*/fan*_input"))
	if err != nil {
		return false
	}
	for _, p := range matches {
		if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() {
			return true
		}
	}
	return false
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func decodeFanRPM(highByte int, lowByte int) int {
	return highByte + (lowByte << 8)
}

func readECFans() FanData {
	base := FanData{ECIOPath: ecIOPath}

	if _, err := os.Stat(ecIOPath); os.IsNotExist(err) {
		msg := "EC IO path not found. Try loading ec_sys and mounting debugfs."
		base.Error = &msg
		return base
	}

	data, err := os.ReadFile(ecIOPath)
	if err != nil {
		if os.IsPermission(err) {
			msg := "Permission denied reading EC IO. Run as root or configure read-only access carefully."
			base.Error = &msg
		} else {
			msg := fmt.Sprintf("Failed to read EC IO: %v", err)
			base.Error = &msg
		}
		return base
	}

	if len(data) < 0x39 {
		msg := fmt.Sprintf("EC IO too short: %d bytes", len(data))
		base.Error = &msg
		return base
	}

	fcmo := int(data[0x31])
	fcmi := int(data[0x32])
	fan1Control := int(data[0x33])
	fan2Control := int(data[0x34])

	fn1l := int(data[0x35])
	fn1h := int(data[0x36])
	fn2l := int(data[0x37])
	fn2h := int(data[0x38])

	fan1RPM := decodeFanRPM(fn1h, fn1l)
	fan2RPM := decodeFanRPM(fn2h, fn2l)

	hexParts := make([]string, 0, 8)
	for i := 0x31; i <= 0x38; i++ {
		hexParts = append(hexParts, fmt.Sprintf("%02x", data[i]))
	}

	return FanData{
		Readable:      true,
		RPMAvailable:  true,
		ECIOPath:      ecIOPath,
		Source:        "acpi_ec",
		Fan1RPM:       fan1RPM,
		Fan2RPM:       fan2RPM,
		Fan1Available: fan1RPM > 0,
		Fan2Available: fan2RPM > 0,
		Mode:          &FanMode{FCMO0x31: fcmo, FCMI0x32: fcmi},
		ControlBytesReadOnly: &FanControlReadOnly{
			Fan10x33: fan1Control,
			Fan20x34: fan2Control,
		},
		Raw: &FanRaw{
			EC0x35FN1L:    fn1l,
			EC0x36FN1H:    fn1h,
			EC0x37FN2L:    fn2l,
			EC0x38FN2H:    fn2h,
			EC0x310x38Hex: strings.Join(hexParts, " "),
		},
		Formula: &FanFormula{
			Fan1: "EC[0x36] + (EC[0x35] << 8)",
			Fan2: "EC[0x38] + (EC[0x37] << 8)",
		},
	}
}

func parseMilliCelsius(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	milli, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	celsius := milli / 1000.0
	if celsius <= 0 || celsius >= 130 {
		return 0, false
	}
	return celsius, true
}

func collectThermalFromDir(root string) ThermalData {
	matches, err := filepath.Glob(filepath.Join(root, "hwmon*", "temp*_input"))
	if err != nil {
		return ThermalData{ControlTempSource: controlTempSourceUnavailable}
	}

	var thermal ThermalData
	var preferredReadable bool
	var preferredMax float64
	for _, p := range matches {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		temp, ok := parseMilliCelsius(string(data))
		if !ok {
			continue
		}
		thermal.SensorCount++
		if !thermal.Readable || temp > thermal.MaxTempCelsius {
			thermal.MaxTempCelsius = temp
		}
		thermal.Readable = true

		chip := stringOr(readText(filepath.Join(filepath.Dir(p), "name")), "unknown")
		if isPreferredControlChip(chip) {
			if !preferredReadable || temp > preferredMax {
				preferredMax = temp
			}
			preferredReadable = true
		}
	}
	if preferredReadable {
		thermal.ControlTempReadable = true
		thermal.ControlTempCelsius = preferredMax
		thermal.ControlTempSource = controlTempSourcePreferred
	} else if thermal.Readable {
		thermal.ControlTempReadable = true
		thermal.ControlTempCelsius = thermal.MaxTempCelsius
		thermal.ControlTempSource = controlTempSourceFallbackMax
	} else {
		thermal.ControlTempSource = controlTempSourceUnavailable
	}
	return thermal
}

func isPreferredControlChip(chip string) bool {
	switch chip {
	case "k10temp", "amdgpu", "acpitz":
		return true
	default:
		return false
	}
}

func collectThermal() ThermalData {
	return collectThermalFromDir(hwmonDir)
}

func evaluateFanStatus(f FanData, t ThermalData) FanStatus {
	if !f.Readable || !f.RPMAvailable {
		return FanStatus{
			Fan1State:  fanStateUnreadable,
			ReasonCode: fanStateUnreadable,
			Reason:     "fan RPM is unreadable; instant sample only",
		}
	}
	if f.Fan1RPM > 0 {
		return FanStatus{
			Fan1State:  fanStateNormal,
			ReasonCode: fanStateNormal,
			Reason:     "fan1_rpm > 0; instant sample only",
		}
	}
	if !t.ControlTempReadable {
		return FanStatus{
			Fan1State:  fanStateFanZeroTempUnknown,
			ReasonCode: fanStateFanZeroTempUnknown,
			Reason:     "fan1_rpm is 0 but thermal data is unavailable; instant sample only",
		}
	}
	if t.ControlTempCelsius < fanStopTempC {
		return FanStatus{
			Fan1State:  fanStateFanStopLowTemp,
			ReasonCode: fanStateFanStopLowTemp,
			Reason:     "fan1_rpm is 0 and control_temp_celsius < 45; instant sample only",
		}
	}
	if t.ControlTempCelsius < fanCriticalTempC {
		return FanStatus{
			Fan1State:  fanStateSuspiciousZeroRPM,
			ReasonCode: fanStateSuspiciousZeroRPM,
			Reason:     "fan1_rpm is 0 and 45 <= control_temp_celsius < 65; instant sample only",
		}
	}
	return FanStatus{
		Fan1State:  fanStateCriticalZeroRPMHighTemp,
		ReasonCode: fanStateCriticalZeroRPMHighTemp,
		Reason:     "fan1_rpm is 0 and control_temp_celsius >= 65; instant sample only",
	}
}

func snapshot() Snapshot {
	fan := readECFans()
	thermal := collectThermal()
	return Snapshot{
		Tool:    "ip3-ec-fan-rpm",
		Version: "0.1.0",
		Mode:    "read_only",
		Safety: Safety{
			WritesEC:               false,
			RequiresECWriteSupport: false,
			ControlsFan:            false,
			ChangesPowerProfile:    false,
		},
		DMI: readDMI(),
		Signals: Signals{
			IP3PowerSwitch:        hasACPIUID("IP3POWERSWITCH"),
			IP3WMIEvent:           hasACPIUID("IP3WMIEVENT"),
			RWECRegWMI:            hasACPIUID("RWECREGWMI"),
			WMIGUID99D89064:       hasWMIGUID("99D89064-8D50-42BB-BEA9-155B2E5D0FCD"),
			WMIGUID8FAFC061:       hasWMIGUID("8FAFC061-22DA-46E2-91DB-1FE3D7E5FF3C"),
			StandardHwmonFanAvail: standardHwmonFanAvailable(),
			ECIOExists:            fileExists(ecIOPath),
		},
		Fan:     fan,
		Thermal: thermal,
		Status:  evaluateFanStatus(fan, thermal),
	}
}

func stringOr(s *string, fallback string) string {
	if s == nil || *s == "" {
		return fallback
	}
	return *s
}

func printHuman(s Snapshot) {
	d := s.DMI
	fmt.Printf("%s %s\n", stringOr(d.SysVendor, "unknown"), stringOr(d.ProductName, "unknown"))
	fmt.Printf("BIOS: %s (%s)\n", stringOr(d.BiosVersion, "unknown"), stringOr(d.BiosDate, "unknown"))
	fmt.Printf("mode: %s\n", s.Mode)

	fmt.Println()
	fmt.Println("signals:")
	fmt.Printf("  ip3_power_switch: %v\n", s.Signals.IP3PowerSwitch)
	fmt.Printf("  ip3_wmi_event: %v\n", s.Signals.IP3WMIEvent)
	fmt.Printf("  rwec_reg_wmi: %v\n", s.Signals.RWECRegWMI)
	fmt.Printf("  wmi_guid_99d89064: %v\n", s.Signals.WMIGUID99D89064)
	fmt.Printf("  wmi_guid_8fafc061: %v\n", s.Signals.WMIGUID8FAFC061)
	fmt.Printf("  standard_hwmon_fan_available: %v\n", s.Signals.StandardHwmonFanAvail)
	fmt.Printf("  ec_io_exists: %v\n", s.Signals.ECIOExists)

	fmt.Println()
	f := s.Fan
	if f.RPMAvailable {
		fmt.Printf("fan1: %d rpm\n", f.Fan1RPM)
		fmt.Printf("fan2: %d rpm\n", f.Fan2RPM)
		if f.Raw != nil {
			fmt.Printf("raw:  %s\n", f.Raw.EC0x310x38Hex)
		}
		fmt.Println("source: ACPI EC, read-only")
	} else {
		errMsg := "unknown error"
		if f.Error != nil {
			errMsg = *f.Error
		}
		fmt.Printf("fan rpm unavailable: %s\n", errMsg)
	}
	if s.Thermal.Readable {
		fmt.Printf("max temp: %.1f °C\n", s.Thermal.MaxTempCelsius)
	} else {
		fmt.Println("max temp: unavailable")
	}
	if s.Thermal.ControlTempReadable {
		fmt.Printf("control temp: %.1f °C (%s)\n", s.Thermal.ControlTempCelsius, s.Thermal.ControlTempSource)
	} else {
		fmt.Println("control temp: unavailable")
	}
	fmt.Printf("fan1 status: %s\n", s.Status.Fan1State)
}

func prometheusEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}

func toPrometheus(s Snapshot) string {
	var b strings.Builder

	vendor := stringOr(s.DMI.SysVendor, "unknown")
	product := stringOr(s.DMI.ProductName, "unknown")
	baseLabels := fmt.Sprintf(`vendor="%s",product="%s"`,
		prometheusEscape(vendor), prometheusEscape(product))

	rpma := 0
	if s.Fan.RPMAvailable {
		rpma = 1
	}

	b.WriteString("# HELP ip3_ec_readable Whether EC fan registers are readable\n")
	b.WriteString("# TYPE ip3_ec_readable gauge\n")
	fmt.Fprintf(&b, "ip3_ec_readable{%s} %d\n", baseLabels, rpma)

	b.WriteString("# HELP ip3_ec_signal_present Whether expected IP3 ACPI/WMI signals are present\n")
	b.WriteString("# TYPE ip3_ec_signal_present gauge\n")

	names := []struct {
		key   string
		value bool
	}{
		{"ip3_power_switch", s.Signals.IP3PowerSwitch},
		{"ip3_wmi_event", s.Signals.IP3WMIEvent},
		{"rwec_reg_wmi", s.Signals.RWECRegWMI},
		{"wmi_guid_99d89064", s.Signals.WMIGUID99D89064},
		{"wmi_guid_8fafc061", s.Signals.WMIGUID8FAFC061},
	}
	for _, n := range names {
		v := 0
		if n.value {
			v = 1
		}
		fmt.Fprintf(&b, "ip3_ec_signal_present{%s,signal=\"%s\"} %d\n",
			baseLabels, prometheusEscape(n.key), v)
	}

	b.WriteString("# HELP ip3_ec_standard_hwmon_fan_available Whether fan RPM is exposed through standard Linux hwmon\n")
	b.WriteString("# TYPE ip3_ec_standard_hwmon_fan_available gauge\n")
	hwav := 0
	if s.Signals.StandardHwmonFanAvail {
		hwav = 1
	}
	fmt.Fprintf(&b, "ip3_ec_standard_hwmon_fan_available{%s} %d\n", baseLabels, hwav)

	b.WriteString("# HELP ip3_ec_fan_rpm Fan RPM read from ACPI EC\n")
	b.WriteString("# TYPE ip3_ec_fan_rpm gauge\n")
	if s.Fan.RPMAvailable {
		fmt.Fprintf(&b, "ip3_ec_fan_rpm{%s,fan=\"1\",source=\"acpi_ec\"} %d\n",
			baseLabels, s.Fan.Fan1RPM)
		fmt.Fprintf(&b, "ip3_ec_fan_rpm{%s,fan=\"2\",source=\"acpi_ec\"} %d\n",
			baseLabels, s.Fan.Fan2RPM)
	}

	b.WriteString("# HELP ip3_ec_thermal_readable Whether hwmon temperature data is readable\n")
	b.WriteString("# TYPE ip3_ec_thermal_readable gauge\n")
	thermReadable := 0
	if s.Thermal.Readable {
		thermReadable = 1
	}
	fmt.Fprintf(&b, "ip3_ec_thermal_readable{%s} %d\n", baseLabels, thermReadable)

	b.WriteString("# HELP ip3_ec_thermal_sensor_count Number of valid hwmon temperature sensors read\n")
	b.WriteString("# TYPE ip3_ec_thermal_sensor_count gauge\n")
	fmt.Fprintf(&b, "ip3_ec_thermal_sensor_count{%s} %d\n", baseLabels, s.Thermal.SensorCount)

	b.WriteString("# HELP ip3_ec_max_temp_celsius Maximum readable hwmon temperature in Celsius\n")
	b.WriteString("# TYPE ip3_ec_max_temp_celsius gauge\n")
	fmt.Fprintf(&b, "ip3_ec_max_temp_celsius{%s} %.1f\n", baseLabels, s.Thermal.MaxTempCelsius)

	source := prometheusEscape(s.Thermal.ControlTempSource)
	b.WriteString("# HELP ip3_ec_control_temp_celsius Temperature used for fan status decisions in Celsius\n")
	b.WriteString("# TYPE ip3_ec_control_temp_celsius gauge\n")
	fmt.Fprintf(&b, "ip3_ec_control_temp_celsius{%s,source=\"%s\"} %.1f\n",
		baseLabels, source, s.Thermal.ControlTempCelsius)

	b.WriteString("# HELP ip3_ec_control_temp_readable Whether the fan status control temperature is readable\n")
	b.WriteString("# TYPE ip3_ec_control_temp_readable gauge\n")
	controlReadable := 0
	if s.Thermal.ControlTempReadable {
		controlReadable = 1
	}
	fmt.Fprintf(&b, "ip3_ec_control_temp_readable{%s,source=\"%s\"} %d\n",
		baseLabels, source, controlReadable)

	b.WriteString("# HELP ip3_ec_fan_status Temperature-aware fan1 status as a one-hot gauge\n")
	b.WriteString("# TYPE ip3_ec_fan_status gauge\n")
	for _, state := range fanStatusStates() {
		v := 0
		if s.Status.Fan1State == state {
			v = 1
		}
		fmt.Fprintf(&b, "ip3_ec_fan_status{%s,status=\"%s\"} %d\n",
			baseLabels, prometheusEscape(state), v)
	}

	return b.String()
}

func fanStatusStates() []string {
	return []string{
		fanStateUnreadable,
		fanStateNormal,
		fanStateFanStopLowTemp,
		fanStateSuspiciousZeroRPM,
		fanStateCriticalZeroRPMHighTemp,
		fanStateFanZeroTempUnknown,
	}
}

func toJSON(s Snapshot) (string, error) {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data) + "\n", nil
}

func writeTextfile(path string, content string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	base := filepath.Base(path)
	f, err := os.CreateTemp(dir, "."+base+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := f.Name()

	cleanup := func() {
		f.Close()
		os.Remove(tmpName)
	}

	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	if _, err := f.WriteString(content); err != nil {
		cleanup()
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := f.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Chmod(tmpName, 0644); err != nil {
		cleanup()
		return fmt.Errorf("chmod temp file: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename temp to target: %w", err)
	}

	return nil
}

func main() {
	jsonFlag := flag.Bool("json", false, "Print JSON output")
	prometheusFlag := flag.Bool("prometheus", false, "Print Prometheus text output")
	textfileFlag := flag.String("textfile", "", "Write Prometheus metrics to a node_exporter textfile collector path")
	watchFlag := flag.Float64("watch", 0, "Repeat every N seconds")
	flag.Parse()

	for {
		s := snapshot()

		switch {
		case *textfileFlag != "":
			prom := toPrometheus(s)
			if err := writeTextfile(*textfileFlag, prom); err != nil {
				fmt.Fprintf(os.Stderr, "textfile error: %v\n", err)
				os.Exit(1)
			}
		case *prometheusFlag:
			fmt.Print(toPrometheus(s))
		case *jsonFlag:
			j, err := toJSON(s)
			if err != nil {
				fmt.Fprintf(os.Stderr, "json error: %v\n", err)
				os.Exit(1)
			}
			fmt.Print(j)
		default:
			printHuman(s)
		}

		if *watchFlag <= 0 {
			break
		}
		time.Sleep(time.Duration(*watchFlag * float64(time.Second)))
	}
}
