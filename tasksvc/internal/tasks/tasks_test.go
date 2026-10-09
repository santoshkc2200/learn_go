package tasks

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeTitle(t *testing.T) {
	cases := []struct {
		name, input, want string
		wantErr           error
	}{
		{"trim", "  learn SQL\t", "learn SQL", nil},
		{"empty", " \n\t", "", ErrInvalidTitle},
		{"limit", strings.Repeat("界", 200), strings.Repeat("界", 200), nil},
		{"too long", strings.Repeat("界", 201), "", ErrInvalidTitle},
		{"invalid UTF8", string([]byte{0xff}), "", ErrInvalidTitle},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeTitle(tc.input)
			if got != tc.want || !errors.Is(err, tc.wantErr) {
				t.Fatalf("got %q, %v; want %q, %v", got, err, tc.want, tc.wantErr)
			}
		})
	}
}
