package clipboard

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestEncode(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"ascii", "hello"},
		{"cjk", "你好世界"},
		{"newline", "line1\nline2"},
		{"mixed", "a你b\nc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Encode(tt.in)
			if !strings.HasPrefix(got, "\x1b]52;c;") {
				t.Fatalf("missing OSC52 prefix: %q", got)
			}
			if !strings.HasSuffix(got, "\x07") {
				t.Fatalf("missing BEL terminator: %q", got)
			}
			payload := strings.TrimSuffix(strings.TrimPrefix(got, "\x1b]52;c;"), "\x07")
			decoded, err := base64.StdEncoding.DecodeString(payload)
			if err != nil {
				t.Fatalf("payload not valid base64: %v", err)
			}
			if string(decoded) != tt.in {
				t.Errorf("round-trip = %q, want %q", decoded, tt.in)
			}
		})
	}
}
