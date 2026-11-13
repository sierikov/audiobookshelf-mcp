package buildinfo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVersionFrom(t *testing.T) {
	tests := []struct {
		name    string
		ldflags string
		main    string // debug.BuildInfo.Main.Version; "" means no build info
		want    string
	}{
		{"ldflags win", "0.3.0", "v0.2.0", "0.3.0"},
		{"ldflags with v", "v0.3.0", "", "0.3.0"},
		{"go install tag", "", "v0.2.0", "0.2.0"},
		{"local build pseudo-version", "", "v0.2.1-0.20261101120000-abcdef123456+dirty", "0.2.1-0.20261101120000-abcdef123456+dirty"},
		{"devel", "", "(devel)", "dev"},
		{"no build info", "", "", "dev"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, versionFrom(tt.ldflags, tt.main))
		})
	}
}

func TestIsRelease(t *testing.T) {
	for _, v := range []string{"0.2.0", "1.10.3"} {
		assert.True(t, IsRelease(v), v)
	}
	for _, v := range []string{
		"dev",
		"",
		"v0.2.0",
		"0.3.0-rc.1",
		"0.2.0+dirty",
		"0.2.1-0.20261101120000-abcdef123456",
		"0.2",
	} {
		assert.False(t, IsRelease(v), v)
	}
}
