package multimon

import "testing"

func TestIsSelfMessage(t *testing.T) {
	m := &Multimon{
		stationCallsign: "N7RIX-10",
	}

	tests := []struct {
		name string
		msg  string
		want bool
	}{
		{
			name: "exact match",
			msg:  "N7RIX-10>APRS:payload",
			want: true,
		},
		{
			name: "lowercase source",
			msg:  "n7rix-10>APRS:payload",
			want: true,
		},
		{
			name: "different callsign",
			msg:  "OTHER-1>APRS:payload",
			want: false,
		},
		{
			name: "missing source delimiter",
			msg:  "INVALID FRAME",
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got := m.isSelfMessage(tt.msg)
			if got != tt.want {
				t.Fatalf("isSelfMessage(%q) = %v, want %v", tt.msg, got, tt.want)
			}
		})
	}
}
