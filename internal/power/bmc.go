package power

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
)

type BMCReader struct {
	verbose        bool
	useSystemPower bool
}

func NewBMCReader(useSystemPower bool, verbose bool) *BMCReader {
	return &BMCReader{
		useSystemPower: useSystemPower,
		verbose:        verbose,
	}
}

func (b *BMCReader) ReadPower(resourceType string) (float64, error) {
	if b.useSystemPower {
		return b.ReadSystemPower()
	}

	switch resourceType {
	case "cpu":
		return b.readCPUPower()
	case "memory":
		return b.readDRAMPower()
	default:
		return b.ReadSystemPower()
	}
}

func (b *BMCReader) ReadSystemPower() (float64, error) {
	cmd := exec.Command("ipmitool", "sdr", "elist", "all")
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to read BMC data: %w", err)
	}

	re := regexp.MustCompile(`HSC Input Power.*?(\d+(?:\.\d+)?)\s*Watts`)
	matches := re.FindStringSubmatch(string(output))
	if len(matches) < 2 {
		return 0, fmt.Errorf("HSC Input Power not found in BMC output")
	}

	power, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse system power value: %w", err)
	}

	return power, nil
}

func (b *BMCReader) readCPUPower() (float64, error) {
	cmd := exec.Command("ipmitool", "sdr", "elist", "all")
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to read BMC data: %w", err)
	}

	re := regexp.MustCompile(`CPU Pkg Power.*?(\d+(?:\.\d+)?)\s*Watts`)
	matches := re.FindStringSubmatch(string(output))
	if len(matches) < 2 {
		return 0, fmt.Errorf("CPU Pkg Power not found in BMC output")
	}

	power, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse CPU power value: %w", err)
	}

	return power, nil
}

func (b *BMCReader) readDRAMPower() (float64, error) {
	cmd := exec.Command("ipmitool", "sdr", "elist", "all")
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to read BMC data: %w", err)
	}

	re := regexp.MustCompile(`DIMM.*POUT.*?(\d+(?:\.\d+)?)\s*Watts`)
	matches := re.FindAllStringSubmatch(string(output), -1)
	if len(matches) == 0 {
		return 0, fmt.Errorf("DRAM power not found in BMC output")
	}

	var sum float64
	var count int
	for _, m := range matches {
		v, err := strconv.ParseFloat(m[1], 64)
		if err == nil {
			sum += v
			count++
		}
	}

	if count == 0 {
		return 0, fmt.Errorf("no parsable DIMM POUT values")
	}

	return sum / float64(count), nil
}
