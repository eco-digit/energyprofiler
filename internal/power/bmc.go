package power

import (
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/eco-digit/energyprofiler/internal/types"
)

type BMCReader struct {
	verbose        bool
	useSystemPower bool
	warnedOnce     map[string]bool
	sensorsLogged  map[string]bool
}

func NewBMCReader(useSystemPower bool, verbose bool) *BMCReader {
	return &BMCReader{
		useSystemPower: useSystemPower,
		verbose:        verbose,
		warnedOnce:     make(map[string]bool),
		sensorsLogged:  make(map[string]bool),
	}
}

func (b *BMCReader) ReadPower(resourceType string) (float64, error) {
	rt := strings.ToLower(resourceType)

	if b.useSystemPower {
		return b.ReadSystemPower()
	}

	// Try component-specific power, with fallback to system if sensor is not available / or regex fails
	switch rt {
	case "cpu":
		return b.readWithFallback("cpu", b.readCPUPower)
	case "memory":
		return b.readWithFallback("memory", b.readDRAMPower)
	case "storage":
		return b.readWithFallback("storage", b.readStoragePower)
	case "system":
		return b.ReadSystemPower()
	default:
		b.logOnce(rt, "unknown component %q, using system power", resourceType)
		return b.ReadSystemPower()
	}
}

// readWithFallback tries component power, falls back to system and warns
func (b *BMCReader) readWithFallback(component string, reader func() (float64, error)) (float64, error) {
	v, err := reader()
	if err == nil {
		return v, nil
	}

	b.logOnce(component, "component power (%s) unavailable: %v, using system power", component, err)
	return b.ReadSystemPower()
}

// logOnce logs a message only once per key to avoid spam
func (b *BMCReader) logOnce(key, format string, args ...any) {
	if !b.warnedOnce[key] {
		b.warnedOnce[key] = true
		log.Printf("[bmc] "+format, args...)
	}
}

// logSensorOnce logs discovered sensor only once per component
func (b *BMCReader) logSensorOnce(component, sensorInfo string) {
	key := "sensor_" + component
	if !b.sensorsLogged[key] {
		b.sensorsLogged[key] = true
		log.Printf("[bmc] Found %s sensor: %s", component, sensorInfo)
	}
}

// ReadSystemPower uses ipmitool DCMI.
// Prefer instantaneous; if missing, fall back to average.
func (b *BMCReader) ReadSystemPower() (float64, error) {
	out, err := exec.Command("ipmitool", "dcmi", "power", "reading").Output()
	if err != nil {
		return 0, fmt.Errorf("ipmitool dcmi power reading failed: %w", err)
	}

	s := string(out)

	// Try instantaneous first
	if v, ok := extractFloat(s, `(?i)Instantaneous\s+power\s+reading:\s*([\d.]+)\s*W`); ok {
		if b.verbose {
			log.Printf("[bmc] System power (instantaneous): %.2f W", v)
		}
		return v, nil
	}

	// Fall back to average
	if v, ok := extractFloat(s, `(?i)Average\s+power\s+reading.*?:\s*([\d.]+)\s*W`); ok {
		if b.verbose {
			log.Printf("[bmc] System power (average): %.2f W", v)
		}
		return v, nil
	}

	return 0, fmt.Errorf("no parsable value in DCMI output")
}

// Component readers (via SDR)

// readCPUPower sums all CPU-related power sensors
func (b *BMCReader) readCPUPower() (float64, error) {
	sdr, err := b.getSDR()
	if err != nil {
		return 0, err
	}

	// Patterns to match CPU power sensors
	patterns := []string{
		`(?i)(CPU\s*Pkg\s*Power).*?([\d.]+)\s*W`,
		`(?i)(CPU\s*Package\s*Power).*?([\d.]+)\s*W`,
		`(?i)(P\d+\s*Package\s*Power).*?([\d.]+)\s*W`,
		`(?i)(Package\s*Power).*?([\d.]+)\s*W`,
		`(?i)(CPU\d?\s*VR\s*POUT).*?([\d.]+)\s*W`,
		`(?i)(P\d+\s*core\s*VR\s*POUT).*?([\d.]+)\s*W`,
	}

	if sum, count, names := sumMatchesWithNames(sdr, patterns...); count > 0 {
		b.logSensorOnce("CPU", names)
		if b.verbose {
			log.Printf("[bmc] CPU power: %.2f W (%d sensors)", sum, count)
		}
		return sum, nil
	}
	return 0, fmt.Errorf("no CPU power sensors found")
}

// readDRAMPower sums all memory power sensors
func (b *BMCReader) readDRAMPower() (float64, error) {
	sdr, err := b.getSDR()
	if err != nil {
		return 0, err
	}

	patterns := []string{
		`(?i)(DIMM.*?VR\d*\s*POUT).*?([\d.]+)\s*W`,
		`(?i)(MEM.*?VR\d*\s*POUT).*?([\d.]+)\s*W`,
		`(?i)(P\d+\s*DIMM.*?VR\d*\s*POUT).*?([\d.]+)\s*W`,
	}

	if sum, count, names := sumMatchesWithNames(sdr, patterns...); count > 0 {
		b.logSensorOnce("DRAM", names)
		if b.verbose {
			log.Printf("[bmc] DRAM power: %.2f W (%d sensors)", sum, count)
		}
		return sum, nil
	}

	return 0, fmt.Errorf("no DRAM power sensors found")
}

// readStoragePower looks for storage-related power sensors
func (b *BMCReader) readStoragePower() (float64, error) {
	sdr, err := b.getSDR()
	if err != nil {
		return 0, err
	}

	patterns := []string{
		`(?i)(NVMe\d*.*?(?:POUT|Power)).*?([\d.]+)\s*W`,
		`(?i)((?:SSD|HDD|SATA).*?(?:POUT|Power)).*?([\d.]+)\s*W`,
		`(?i)((?:Drive|Disk).*?(?:POUT|Power)).*?([\d.]+)\s*W`,
		`(?i)(Storage.*?(?:POUT|Power)).*?([\d.]+)\s*W`,
	}

	if sum, count, names := sumMatchesWithNames(sdr, patterns...); count > 0 {
		b.logSensorOnce("Storage", names)
		if b.verbose {
			log.Printf("[bmc] Storage power: %.2f W (%d sensors)", sum, count)
		}
		return sum, nil
	}
	return 0, fmt.Errorf("no storage power sensors found")
}

// --- Helpers ---

// getSDR runs ipmitool sdr once and returns output
func (b *BMCReader) getSDR() (string, error) {
	out, err := exec.Command("ipmitool", "sdr", "elist", "all").Output()
	if err != nil {
		return "", fmt.Errorf("ipmitool sdr elist failed: %w", err)
	}
	return string(out), nil
}

// extractFloat finds first match of pattern and extracts float from capture group 1
func extractFloat(text, pattern string) (float64, bool) {
	m := regexp.MustCompile(pattern).FindStringSubmatch(text)
	if len(m) < 2 {
		return 0, false
	}

	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, false
	}

	return v, true
}

// sumMatchesWithNames sums matches and returns sensor names found
func sumMatchesWithNames(text string, patterns ...string) (sum float64, count int, names string) {
	var foundSensors []string

	for _, p := range patterns {
		re := regexp.MustCompile(p)
		for _, m := range re.FindAllStringSubmatch(text, -1) {
			if len(m) >= 3 {
				// m[1] is sensor name, m[2] is value
				if v, err := strconv.ParseFloat(m[2], 64); err == nil {
					sum += v
					count++
					// Collect first 3 sensor names
					if len(foundSensors) < 3 {
						foundSensors = append(foundSensors, strings.TrimSpace(m[1]))
					}
				}
			}
		}
	}

	// Format names string
	if len(foundSensors) > 0 {
		names = strings.Join(foundSensors, ", ")

		if count > len(foundSensors) {
			names = fmt.Sprintf("%s (and %d more)", strings.Join(foundSensors, ", "), count-len(foundSensors))
		}
	}

	return sum, count, names
}

// compile time compliance check
var _ types.PowerReader = (*BMCReader)(nil)
