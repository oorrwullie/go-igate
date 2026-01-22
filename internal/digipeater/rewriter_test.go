package digipeater

import (
	"testing"

	"github.com/oorrwullie/go-igate/internal/aprs"
	"github.com/oorrwullie/go-igate/internal/config"
)

func TestRewriter(t *testing.T) {
	tests := []struct {
		name        string
		frame       string
		wantPath    []string
		wantRewrite bool
	}{
		{
			name:        "rewrite-wide1-1",
			frame:       "CALL1>APRS,WIDE1-1:/123456h4903.50N/07201.75W-Test message",
			wantPath:    []string{"N0CALL-1*"},
			wantRewrite: true,
		},
		{
			name:        "skip-self",
			frame:       "N0CALL-1>APRS,WIDE1-1:/123456h4903.50N/07201.75W-Test message",
			wantPath:    []string{"WIDE1-1"},
			wantRewrite: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rewriter, err := NewRewriter("N0CALL-1", config.Digipeater{
				AliasPatterns: []string{`^WIDE1-1$`},
				WidePatterns:  []string{`^WIDE[1-7]-[1-7]$`},
			})
			if err != nil {
				t.Fatalf("failed to create rewriter: %v", err)
			}

			packet, err := aprs.ParsePacket(tt.frame)
			if err != nil {
				t.Fatalf("failed to parse packet: %v", err)
			}

			gotRewrite := rewriter.Rewrite(packet)
			if gotRewrite != tt.wantRewrite {
				t.Fatalf("unexpected rewrite result: %v", gotRewrite)
			}

			if len(packet.Path) != len(tt.wantPath) {
				t.Fatalf("unexpected rewritten path: %#v", packet.Path)
			}
			for idx, want := range tt.wantPath {
				if packet.Path[idx] != want {
					t.Fatalf("unexpected rewritten path: %#v", packet.Path)
				}
			}
		})
	}
}
