package config

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func DetectDataPort() (string, error) {
	var (
		productIDs    = []string{"2303", "7523"}
		vendorIDs     = []string{"067b", "1a86"}
		cmd           *exec.Cmd
		commandOutput string
		productMatch  bool
		vendorMatch   bool
	)

	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("lsusb", "-v")

		out, err := cmd.Output()
		if err != nil {
			return "", err
		}

		commandOutput = string(out)
		lines := strings.Split(commandOutput, "\n")

		for _, line := range lines {
			if containsAny(line, productIDs) {
				productMatch = true
			}

			if productMatch && containsAny(line, vendorIDs) {
				vendorMatch = true
			} else {
				productMatch = false

				continue
			}

			if productMatch && vendorMatch && strings.HasPrefix(line, "Bus") {
				files, _ := filepath.Glob("/dev/ttyUSB*")
				if len(files) > 0 {
					return files[0], nil
				} else {
					return "", fmt.Errorf("no USB data cable found")
				}
			}
		}
	case "darwin":
		cmd = exec.Command("system_profiler", "SPUSBDataType")

		out, err := cmd.Output()
		if err != nil {
			return "", err
		}

		commandOutput = string(out)
		lines := strings.Split(commandOutput, "\n")

		for i, line := range lines {
			var nextLine string
			if containsAny(line, productIDs) {
				productMatch = true
				nextLine = lines[i+1]
			}

			if productMatch && (containsAny(nextLine, vendorIDs) || containsAny(nextLine, vendorIDs)) {
				vendorMatch = true
			} else {
				productMatch = false

				continue
			}

			if productMatch && vendorMatch {
				files, _ := filepath.Glob("/dev/tty.usbserial*")
				if len(files) > 0 {
					return files[0], nil
				} else {
					return "", fmt.Errorf("no USB data cable found")
				}
			}
		}
	default:
		return "", fmt.Errorf("unsupported platform")
	}

	return "", fmt.Errorf("no USB device found")
}

func containsAny(s string, substrings []string) bool {
	for _, substr := range substrings {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}
