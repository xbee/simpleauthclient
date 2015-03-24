package common

import (
	"time"
)

func Timer(interval time.Duration, call func()) {
	go func() {
		t := time.Tick(interval)
		for {
			select {
			case <-t:
				call()
			}
		}
	}()
}
