package power

import (
	"context"
	"testing"
)

func TestParseBusctlString(t *testing.T) {
	cases := map[string]string{
		"s \"balanced\"\n":    "balanced",
		"s \"power-saver\"":   "power-saver",
		"s \"performance\"\n": "performance",
		"":                    "",
	}
	for in, want := range cases {
		if got := parseBusctlString([]byte(in)); got != want {
			t.Errorf("parseBusctlString(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestReadProfileLive(t *testing.T) {
	got := readProfile(context.Background())
	if got == "unknown" {
		t.Skip("power-profiles-daemon not reachable")
	}
	switch got {
	case "balanced", "power-saver", "performance":
	default:
		t.Errorf("unexpected profile %q", got)
	}
	t.Logf("active profile: %s", got)
}
