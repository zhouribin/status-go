package ratelimiter

type Interface interface {
	Create(id []byte) error
	Remove(id []byte) error
	TakeAvailable(id []byte, count int64) int64
	Available(id []byte) int64
	UpdateConfig(id []byte, config Config) error
	Config() Config
}
