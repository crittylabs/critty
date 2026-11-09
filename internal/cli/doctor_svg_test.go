package cli

import (
	"os/exec"
	"strings"
	"testing"
)

func TestDoctorSVGSetupAdvice(t *testing.T) {
	old := lookPath
	t.Cleanup(func() { lookPath = old })
	for _, available := range []string{"", "rsvg-convert", "merman-cli", "resvg"} {
		t.Run("available="+available, func(t *testing.T) {
			lookPath = func(name string) (string, error) {
				if name == available {
					return "/tools/" + name, nil
				}
				return "", exec.ErrNotFound
			}
			var r doctorReport
			r.checkSVG()
			advice := 0
			for _, d := range r.Checks {
				if strings.Contains(d.Advice, "critty install tools svg") {
					advice++
				}
			}
			want := 0
			if available == "" {
				want = 1
			}
			if advice != want {
				t.Fatalf("setup suggestions = %d, want %d: %+v", advice, want, r.Checks)
			}
		})
	}
}
