package capture

import (
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

func TestPrepareMultimonAudioDownmixesAndResamples(t *testing.T) {
	got := prepareMultimonAudio([]float32{1, -1, 0.5, 0.5, 1, 1, -0.5, -0.5}, 2, SampleRate)
	want := []float32{0.25, 0.25}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("prepareMultimonAudio() = %v, want %v", got, want)
	}
}

func TestPrepareMultimonAudioKeeps22050HzMonoRate(t *testing.T) {
	got := prepareMultimonAudio([]float32{0.25, -0.5, 0.75}, 1, SampleRate/2)
	want := []float32{0.25, -0.5, 0.75}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("prepareMultimonAudio() = %v, want %v", got, want)
	}
}
