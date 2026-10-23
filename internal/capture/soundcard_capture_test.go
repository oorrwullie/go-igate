package capture

import (
	"math"
	"reflect"
	"testing"
)

func TestDownmixToMono(t *testing.T) {
	got := downmixToMono([]float32{1, -1, 0.5, 0.5}, 2)
	want := []float32{0, 0.5}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("downmixToMono() = %v, want %v", got, want)
	}
}

func TestMultimonResamplerDownmixesAndResamples(t *testing.T) {
	resampler := newMultimonResampler(2, SampleRate)
	got := resampler.Process([]float32{1, -1, 0.5, 0.5, 1, 1, -0.5, -0.5})
	want := []float32{0.25, 0.25}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Process() = %v, want %v", got, want)
	}
}

func TestMultimonResamplerKeeps22050HzMonoRate(t *testing.T) {
	resampler := newMultimonResampler(1, SampleRate/2)
	got := resampler.Process([]float32{0.25, -0.5, 0.75})
	want := []float32{0.25, -0.5, 0.75}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Process() = %v, want %v", got, want)
	}
}

func TestMultimonResamplerOutputDoesNotDependOnChunkBoundaries(t *testing.T) {
	tests := []struct {
		name      string
		inputRate int
		channels  int
		chunkEnds []int
	}{
		{name: "44100 Hz odd chunks", inputRate: 44100, channels: 1, chunkEnds: []int{3, 8, 11, 16}},
		{name: "48000 Hz uneven stereo chunks", inputRate: 48000, channels: 2, chunkEnds: []int{6, 16, 22, 32}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := make([]float32, 16)
			for i := range input {
				input[i] = float32((i*17)%23-11) / 13
			}
			if tt.channels == 2 {
				stereo := make([]float32, 0, len(input)*2)
				for _, sample := range input {
					stereo = append(stereo, sample, sample/2)
				}
				input = stereo
			}

			whole := newMultimonResampler(tt.channels, tt.inputRate).Process(input)
			chunkedResampler := newMultimonResampler(tt.channels, tt.inputRate)
			var chunked []float32
			start := 0
			for _, end := range tt.chunkEnds {
				chunked = append(chunked, chunkedResampler.Process(input[start:end])...)
				start = end
			}
			if start < len(input) {
				chunked = append(chunked, chunkedResampler.Process(input[start:])...)
			}

			if len(chunked) != len(whole) {
				t.Fatalf("chunked output has %d samples, whole output has %d", len(chunked), len(whole))
			}
			for i := range whole {
				if diff := math.Abs(float64(chunked[i] - whole[i])); diff > 1e-6 {
					t.Fatalf("sample %d differs: chunked %g, whole %g", i, chunked[i], whole[i])
				}
			}
		})
	}
}
