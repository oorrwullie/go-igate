package config

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	productID string = "2303"
	vendorID  string = "067b"
)

func DetectDataPort() (string, error) {
	var (
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
			if strings.Contains(line, productID) {
				productMatch = true
			}

			if productMatch && strings.Contains(line, vendorID) {
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
			if strings.Contains(line, productID) {
				productMatch = true
				nextLine = lines[i+1]
			}

			if productMatch && (strings.Contains(nextLine, vendorID) || strings.Contains(nextLine, vendorID)) {
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

	return "", fmt.Errorf("no USB device found with product ID %s and vendor ID %s", productID, vendorID)
}
