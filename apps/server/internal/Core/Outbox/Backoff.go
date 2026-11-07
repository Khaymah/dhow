package outbox

import "time"

func NextDelay(schedule []time.Duration, attempts int) time.Duration {
	if len(schedule) == 0 {
		return 0
	}
	i := attempts - 1
	if i < 0 {
		i = 0
	}
	if i >= len(schedule) {
		i = len(schedule) - 1
	}
	return schedule[i]
}

func Final(attempts, maxAttempts int, res Result) bool {
	return res.Permanent || attempts >= maxAttempts
}
