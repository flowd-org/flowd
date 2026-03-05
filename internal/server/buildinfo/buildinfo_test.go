// SPDX-License-Identifier: AGPL-3.0-or-later

package buildinfo

import (
	"testing"
)

func TestVersion(t *testing.T) {
	tests := []struct {
		name   string
		envVal string
		want   string
	}{
		{
			name:   "FLWD_VERSION unset",
			envVal: "",
			want:   "dev",
		},
		{
			name:   "FLWD_VERSION whitespace-only",
			envVal: "   ",
			want:   "dev",
		},
		{
			name:   "FLWD_VERSION with spaces",
			envVal: " 1.2.3 ",
			want:   "1.2.3",
		},
		{
			name:   "FLWD_VERSION clean",
			envVal: "2.0.0",
			want:   "2.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("FLWD_VERSION", tt.envVal)
			if got := Version(); got != tt.want {
				t.Errorf("Version() = %q, want %q", got, tt.want)
			}
		})
	}
}
