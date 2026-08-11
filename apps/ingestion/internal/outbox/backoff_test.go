package outbox

import (
	"testing"
	"time"
)

func TestNextBackoff(t *testing.T) {
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: 1, want: 1 * time.Second},
		{attempt: 2, want: 5 * time.Second},
		{attempt: 3, want: 30 * time.Second},
		{attempt: 4, want: 2 * time.Minute},
		{attempt: 5, want: 10 * time.Minute},
		{attempt: 6, want: 10 * time.Minute}, // beyond schedule — ceiling applies
		{attempt: 100, want: 10 * time.Minute},
	}

	for _, c := range cases {
		got := nextBackoff(c.attempt)
		if got != c.want {
			t.Errorf("nextBackoff(%d) = %v, want %v", c.attempt, got, c.want)
		}
	}
}
