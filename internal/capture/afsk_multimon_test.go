package capture

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerateAFSKWithMultimonNG verifies that the generated AFSK audio decodes to the original frame.
func TestGenerateAFSKWithMultimonNG(t *testing.T) {
	const aprsFrame = "N0CALL-1>APRS,WIDE1-1:/123456h4903.50N/07201.75W-Test message"

	sc := &SoundcardCapture{}
	ax25, err := sc.aprsToAx25(aprsFrame)
	if err != nil {
		t.Fatalf("aprsToAx25 failed: %v", err)
	}

	wave, err := sc.generateAFSK(ax25)
	if err != nil {
		t.Fatalf("generateAFSK failed: %v", err)
	}

	wavData := buildWav(wave)

	tmpDir := t.TempDir()
	tmpFile, err := os.CreateTemp(tmpDir, "afsk_*.wav")
	if err != nil {
		t.Fatalf("CreateTemp failed: %v", err)
	}
	if _, err := tmpFile.Write(wavData); err != nil {
		t.Fatalf("write temp wav failed: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("close temp wav failed: %v", err)
	}

	cmd := exec.Command("multimon-ng", "-a", "AFSK1200", "-A", "-t", "wav", tmpFile.Name())

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	if err := cmd.Run(); err != nil {
		t.Fatalf("multimon-ng failed: %v (output=%q)", err, output.String())
	}

	var decoded string
	scanner := bufio.NewScanner(&output)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "APRS:") {
			decoded = strings.TrimSpace(strings.TrimPrefix(line, "APRS:"))
			break
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner error: %v", err)
	}

	if decoded == "" {
		_ = os.WriteFile(filepath.Join(tmpDir, "afsk_failure.wav"), wavData, 0o644)
		t.Fatalf("no APRS frame decoded; output=%q", output.String())
	}

	if decoded != aprsFrame {
		t.Fatalf("decoded frame mismatch: got %q, want %q", decoded, aprsFrame)
	}
}

func buildWav(samples []float32) []byte {
	const (
		audioFormat   = 1
		numChannels   = 1
		sampleRate    = SampleRate
		bitsPerSample = 16
		byteRate      = sampleRate * numChannels * bitsPerSample / 8
		blockAlign    = numChannels * bitsPerSample / 8
	)

	var pcm bytes.Buffer
	for _, sample := range samples {
		if sample > 1 {
			sample = 1
		} else if sample < -1 {
			sample = -1
		}
		value := int16(sample * 32767)
		_ = binary.Write(&pcm, binary.LittleEndian, value)
	}

	dataLen := uint32(pcm.Len())
	riffSize := 4 + 8 + 16 + 8 + dataLen

	var wav bytes.Buffer
	wav.WriteString("RIFF")
	_ = binary.Write(&wav, binary.LittleEndian, uint32(riffSize))
	wav.WriteString("WAVE")
	wav.WriteString("fmt ")
	_ = binary.Write(&wav, binary.LittleEndian, uint32(16))
	_ = binary.Write(&wav, binary.LittleEndian, uint16(audioFormat))
	_ = binary.Write(&wav, binary.LittleEndian, uint16(numChannels))
	_ = binary.Write(&wav, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(&wav, binary.LittleEndian, uint32(byteRate))
	_ = binary.Write(&wav, binary.LittleEndian, uint16(blockAlign))
	_ = binary.Write(&wav, binary.LittleEndian, uint16(bitsPerSample))
	wav.WriteString("data")
	_ = binary.Write(&wav, binary.LittleEndian, dataLen)
	wav.Write(pcm.Bytes())

	return wav.Bytes()
}
