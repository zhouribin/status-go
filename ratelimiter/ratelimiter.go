package ratelimiter

import (
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/juju/ratelimit"
	"github.com/status-im/status-go/db"
	"github.com/syndtr/goleveldb/leveldb"
)

type Config struct {
	Capacity uint32
	Quantum  uint32
	Interval uint32
}

// compare config with existing ratelimited bucket.
func compare(c Config, bucket *ratelimit.Bucket) bool {
	return int64(c.Capacity) == bucket.Capacity() &&
		1e9*float64(c.Quantum)/float64(c.Interval) == bucket.Rate()
}

func newBucket(c Config) *ratelimit.Bucket {
	return ratelimit.NewBucketWithQuantum(time.Duration(c.Interval), int64(c.Capacity), int64(c.Quantum))
}

func NewPersisted(db *leveldb.DB, config Config) *PersistedRateLimiter {
	return &PersistedRateLimiter{
		db:            db,
		defaultConfig: config,
		initialized:   map[string]*ratelimit.Bucket{},
	}
}

// PersistedRateLimiter persists latest capacity and updated config per unique ID.
type PersistedRateLimiter struct {
	db            *leveldb.DB
	defaultConfig Config

	mu          sync.Mutex
	initialized map[string]*ratelimit.Bucket
}

func (r *PersistedRateLimiter) getOrCreate(id []byte, config Config) (bucket *ratelimit.Bucket) {
	r.mu.Lock()
	defer r.mu.Unlock()
	old, exist := r.initialized[string(id)]
	if !exist {
		bucket = newBucket(config)
		r.initialized[string(id)] = bucket
	} else {
		bucket = old
	}
	return
}

func (r *PersistedRateLimiter) Create(id []byte) error {
	fkey := db.Key(db.RateLimitConfig, id)
	val, err := r.db.Get(fkey, nil)
	var cfg Config
	if err == leveldb.ErrNotFound {
		cfg = r.defaultConfig
	} else if err != nil {
		return fmt.Errorf("failed to read key %x from database: %v", fkey, err)
	} else {
		if err := rlp.DecodeBytes(val, &cfg); err != nil {
			return fmt.Errorf("failed to decode bytes %x into Config object: %v", val, err)
		}
	}
	bucket := r.getOrCreate(id, cfg)
	fkey = db.Key(db.RateLimitCapacity, id)
	val, err = r.db.Get(fkey, nil)
	if err == leveldb.ErrNotFound {
		return nil
	} else if len(val) != 8 {
		log.Error("stored value is of unexpected length", "expected", 8, "stored", len(val))
		return nil
	}
	bucket.TakeAvailable(int64(binary.BigEndian.Uint32(val[:4])))
	// TODO refill rate limiter due to time difference. e.g. if record was stored at T and C seconds passed since T.
	// we need to add RATE_PER_SECOND*C to a bucket
	return nil
}

// Remove removes key from memory but ensures that the latest information is persisted.
func (r *PersistedRateLimiter) Remove(id []byte) error {
	r.mu.Lock()
	bucket, exist := r.initialized[string(id)]
	delete(r.initialized, string(id))
	r.mu.Unlock()
	if !exist || bucket == nil {
		return nil
	}
	return r.store(id, bucket)
}

func (r *PersistedRateLimiter) store(id []byte, bucket *ratelimit.Bucket) error {
	buf := [8]byte{}
	binary.BigEndian.PutUint32(buf[:], uint32(bucket.Capacity()-bucket.Available()))
	binary.BigEndian.PutUint32(buf[4:], uint32(time.Now().Unix()))
	err := r.db.Put(db.Key(db.RateLimitCapacity, id), buf[:], nil)
	if err != nil {
		return fmt.Errorf("failed to write current capacicity %d for id %x: %v",
			bucket.Capacity(), id, err)
	}
	return nil
}

func (r *PersistedRateLimiter) TakeAvailable(id []byte, count int64) int64 {
	bucket := r.getOrCreate(id, r.defaultConfig)
	rst := bucket.TakeAvailable(count)
	if err := r.store(id, bucket); err != nil {
		log.Error(err.Error())
	}
	return rst
}

func (r *PersistedRateLimiter) Available(id []byte) int64 {
	return r.getOrCreate(id, r.defaultConfig).Available()
}

func (r *PersistedRateLimiter) UpdateConfig(id []byte, config Config) error {
	r.mu.Lock()
	old, _ := r.initialized[string(id)]
	if compare(config, old) {
		r.mu.Unlock()
		return nil
	}
	delete(r.initialized, string(id))
	r.mu.Unlock()
	taken := int64(0)
	if old != nil {
		taken = old.Capacity() - old.Available()
	}
	r.getOrCreate(id, config).TakeAvailable(taken)
	return nil
}
