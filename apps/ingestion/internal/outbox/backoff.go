package outbox

import "time"

// backoffSchedule maps attempt count to the delay before the next retry,
// per ARCHITECTURE.md §4.3. Attempts beyond the schedule's length reuse
// its final (longest) entry as a ceiling rather than growing further or
// erroring.
var backoffSchedule = []time.Duration{
	1 * time.Second,
	5 * time.Second,
	30 * time.Second,
	2 * time.Minute,
	10 * time.Minute,
}

// nextBackoff returns the delay to wait before retrying, given the
// attempt count that has just failed (1 for the first failure, 2 for the
// second, and so on).
func nextBackoff(attemptCount int) time.Duration {
	index := attemptCount - 1
	if index < 0 {
		index = 0
	}
	if index >= len(backoffSchedule) {
		index = len(backoffSchedule) - 1
	}
	return backoffSchedule[index]
}
