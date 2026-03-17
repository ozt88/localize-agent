package shared

import "time"

func CallWithRetry(fn func() (string, error), maxAttempts int, backoffSec float64) (string, error) {
	delay := time.Duration(backoffSec * float64(time.Second))
	if delay <= 0 {
		delay = 100 * time.Millisecond
	}
	var last error
	for i := 1; i <= maxAttempts; i++ {
		out, err := fn()
		if err == nil {
			return out, nil
		}
		last = err
		if i < maxAttempts {
			time.Sleep(delay)
			delay *= 2
		}
	}
	return "", last
}
