package ratelimiter

import "time"

type Interface interface {
	Create([]byte) error
	Remove([]byte, time.Duration) error
	TakeAvailable([]byte, int64) int64
	Available([]byte) int64
	UpdateConfig([]byte, Config) error
	Config() Config
}
