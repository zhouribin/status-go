package ratelimiter

func withPrefix(p []byte, ratelimiter Interface) prefix {
	return prefix{
		prefix:      p,
		prefixLen:   len(p),
		ratelimiter: ratelimiter,
	}
}

type prefix struct {
	prefix      []byte
	prefixLen   int
	ratelimiter Interface
}

func (r prefix) prefixed(id []byte) []byte {
	rst := make([]byte, len(id)+r.prefixLen)
	copy(rst, r.prefix)
	copy(rst[r.prefixLen:], id)
	return rst
}

func (r prefix) Create(id []byte) error {
	return r.ratelimiter.Create(r.prefixed(id))
}

func (r prefix) Remove(id []byte) error {
	return r.ratelimiter.Remove(r.prefixed(id))
}

func (r prefix) TakeAvailable(id []byte, count int64) int64 {
	return r.ratelimiter.TakeAvailable(r.prefixed(id), count)
}

func (r prefix) Available(id []byte) int64 {
	return r.ratelimiter.Available(r.prefixed(id))
}

func (r prefix) UpdateConfig(id []byte, config Config) error {
	return r.ratelimiter.UpdateConfig(r.prefixed(id), config)
}
