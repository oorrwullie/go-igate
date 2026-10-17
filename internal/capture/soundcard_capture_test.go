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
