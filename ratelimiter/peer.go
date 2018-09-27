package ratelimiter

import (
	"net"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/p2p"
	"github.com/syndtr/goleveldb/leveldb"
)

const (
	IDMode = 1 + iota
	IPMode
)

func ipModeFunc(peer *p2p.Peer) []byte {
	addr := peer.RemoteAddr().Network()
	ip := net.ParseIP(strings.Split(addr, ":")[0])
	return []byte(ip)
}

func idModeFunc(peer *p2p.Peer) []byte {
	return peer.ID().Bytes()
}

// selectFunc returns idModeFunc by default.
func selectFunc(mode int) func(*p2p.Peer) []byte {
	if mode == IPMode {
		return ipModeFunc
	}
	return idModeFunc
}

func NewP2PRateLimiter(mode int, ratelimiter Interface) P2PPeerRateLimiter {
	return P2PPeerRateLimiter{
		modeFunc:    selectFunc(mode),
		ratelimiter: ratelimiter,
	}
}

type P2PPeerRateLimiter struct {
	modeFunc    func(*p2p.Peer) []byte
	ratelimiter Interface
}

func (r P2PPeerRateLimiter) Config() Config {
	return r.ratelimiter.Config()
}

func (r P2PPeerRateLimiter) Create(peer *p2p.Peer) error {
	return r.ratelimiter.Create(r.modeFunc(peer))
}

func (r P2PPeerRateLimiter) Remove(peer *p2p.Peer, duration time.Duration) error {
	return r.ratelimiter.Remove(r.modeFunc(peer), duration)
}

func (r P2PPeerRateLimiter) TakeAvailable(peer *p2p.Peer, count int64) int64 {
	return r.ratelimiter.TakeAvailable(r.modeFunc(peer), count)
}

func (r P2PPeerRateLimiter) Available(peer *p2p.Peer) int64 {
	return r.ratelimiter.Available(r.modeFunc(peer))
}

func (r P2PPeerRateLimiter) UpdateConfig(peer *p2p.Peer, config Config) error {
	return r.ratelimiter.UpdateConfig(r.modeFunc(peer), config)
}

type Whisper struct {
	ingress, egress P2PPeerRateLimiter
}

func ForWhisper(mode int, db *leveldb.DB, ingress, egress Config) Whisper {
	return Whisper{
		ingress: NewP2PRateLimiter(mode, NewPersisted(db, ingress, []byte("i"))),
		egress:  NewP2PRateLimiter(mode, NewPersisted(db, egress, []byte("e"))),
	}
}

func (w Whisper) I() P2PPeerRateLimiter {
	return w.ingress
}

func (w Whisper) E() P2PPeerRateLimiter {
	return w.egress
}
