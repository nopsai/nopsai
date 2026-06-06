package resolver

import "time"

func withRetry(op func() error, attempts int, initialBackoff time.Duration, stop func(error) bool) error {
	var err error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			time.Sleep(initialBackoff * time.Duration(1<<uint(i-1)))
		}
		err = op()
		if err == nil {
			return nil
		}
		if stop != nil && stop(err) {
			return err
		}
	}
	return err
}
