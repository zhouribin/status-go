package peers2

import (
	"time"

	"github.com/status-im/status-go/geth/peers2/cache"
)

// Option is a peer pool option passed to the constructor.
type Option func(*PeerPool)

// SetAllowStop sets if the PeerPool can stop Discovery v5.
func SetAllowStop(val bool) Option {
	return func(p *PeerPool) {
		p.allowStop = val
	}
}

// SetDiscoveryTimeout sets a timeout after which Discovery v5 is stopped.
func SetDiscoveryTimeout(timeout time.Duration) Option {
	return func(p *PeerPool) {
		p.discoveryTimeout = timeout
	}
}

// SetTopicSyncPeriod sets a period for running topic search requests.
func SetTopicSyncPeriod(period time.Duration) Option {
	return func(p *PeerPool) {
		p.topicSyncPeriod = period
	}
}

// SetCache sets a cache used by PeerPool and TopicPools.
func SetCache(cache *cache.Cache) Option {
	return func(p *PeerPool) {
		p.cache = cache
	}
}
