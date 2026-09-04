package update_test

import (
	"testing"

	"github.com/JaredTate/nerdgenie/internal/update"
)

func TestOneVersionIsNewerThanAnother(t *testing.T) {
	cases := []struct {
		running   string
		candidate string
		newer     bool
	}{
		{"0.7.0", "0.7.1", true},
		{"0.7.0", "0.8.0", true},
		{"0.7.0", "1.0.0", true},
		{"0.7.0", "0.7.0", false},
		{"0.7.1", "0.7.0", false},
		{"1.0.0", "0.9.9", false},
		{"v0.7.0", "v0.7.1", true},
		{"0.9.0", "0.10.0", true},
		{"0.7.0", "0.7.0-rc1", false},
		{"0.7.0-rc1", "0.7.0", true},
		{"dev", "0.7.0", true},
		{"0.7.0", "dev", false},
		{"dev", "dev", false},
		{"", "0.7.0", true},
	}

	for _, one := range cases {
		if newer := update.Newer(one.running, one.candidate); newer != one.newer {
			t.Errorf("Newer(%q, %q) says %v rather than %v", one.running, one.candidate, newer, one.newer)
		}
	}
}
