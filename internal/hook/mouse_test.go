package hook

import "testing"

// TestInputAgeMillisSurvivesTickWrap keeps the dead-hook watchdog working past 49.7 days of uptime:
// GetLastInputInfo wraps at 2^32 while GetTickCount64 keeps counting.
func TestInputAgeMillisSurvivesTickWrap(t *testing.T) {
	const wrap = 1 << 32
	cases := []struct {
		now       uint64
		lastInput uint32
		want      uint32
	}{
		{1000, 500, 500},
		{wrap + 5, uint32(wrap - 1000), 1005},
		{wrap + 3000, 10, 2990},
	}
	for _, tc := range cases {
		if got := inputAgeMillis(tc.now, tc.lastInput); got != tc.want {
			t.Errorf("inputAgeMillis(%d, %d) = %d, want %d", tc.now, tc.lastInput, got, tc.want)
		}
	}
}
