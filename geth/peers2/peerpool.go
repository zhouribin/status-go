package peers2

import (
	"errors"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/p2p/discv5"
	"github.com/status-im/status-go/geth/params"
	"github.com/status-im/status-go/geth/peers2/cache"
	"github.com/status-im/status-go/geth/peers2/topic"
	"github.com/status-im/status-go/signal"
)

const (
	// DefaultDiscoveryTimeout is a default time after which
	// Discovery is stopped.
	DefaultDiscoveryTimeout = 10 * time.Second

	// DefaultTopicSyncPeriod defines default period of topic sync calls.
	DefaultTopicSyncPeriod = time.Millisecond * 500

	// defaultDiscoveryRestartTimeout defines a duration after which
	// the loop will retry to start the discovery server
	// in case it failed to start previously.
	defaultDiscoveryRestartTimeout = 2 * time.Second
)

var (
	// ErrDiscv5NotRunning returned when pool is started
	// but Discover v5 is not running or not enabled.
	ErrDiscv5NotRunning = errors.New("Discovery v5 is not running")
)

type runtimeOp func(*p2p.Server)

// PeerPool manages topic pools.
type PeerPool struct {
	allowStop        bool
	discoveryTimeout time.Duration
	topicSyncPeriod  time.Duration

	running    bool
	topicPools []*topic.Pool
	cache      *cache.Cache

	peerEvents    chan *p2p.PeerEvent
	peerEventsSub event.Subscription

	mu           sync.RWMutex
	quit         chan struct{}
	wg           sync.WaitGroup
	runtimeOps   chan runtimeOp
	wgRuntimeOps sync.WaitGroup
}

// NewPeerPool creates instance of PeerPool.
func NewPeerPool(opts ...Option) *PeerPool {
	pool := PeerPool{
		discoveryTimeout: DefaultDiscoveryTimeout,
		topicSyncPeriod:  DefaultTopicSyncPeriod,
		runtimeOps:       make(chan runtimeOp),
	}
	for _, opt := range opts {
		opt(&pool)
	}
	return &pool
}

// Start creates topic pools based on the provided config.
// It also starts listening to p2p peer events.
func (p *PeerPool) Start(server *p2p.Server, config map[discv5.Topic]params.Limits) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	defaultDiscV5Sentinel.RLock(server)
	defer defaultDiscV5Sentinel.RUnlock(server)

	if server.DiscV5 == nil {
		return ErrDiscv5NotRunning
	}

	p.running = true
	p.quit = make(chan struct{})

	// handle discovery runtime
	p.wg.Add(1)
	go func() {
		p.handleDiscoveryRuntime(server)
		p.wg.Done()
	}()

	// subscribe to peer events
	p.peerEvents = make(chan *p2p.PeerEvent, 20)
	p.peerEventsSub = server.SubscribeEvents(p.peerEvents)
	p.wg.Add(1)
	go func() {
		p.handlePeerEvents(server, p.peerEvents)
		p.wg.Done()
	}()

	// collect topics and start searching for nodes
	p.topicPools = make([]*topic.Pool, 0, len(config))
	for discv5Topic, limits := range config {
		topicPool := topic.NewPool(discv5Topic, limits, p.cache)
		if err := topicPool.StartSearch(server, p.topicSyncPeriod); err != nil {
			return err
		}
		p.topicPools = append(p.topicPools, topicPool)
	}

	p.runtimeOps <- p.opDiscoveryStarted()

	return nil
}

// startDiscovery starts the Discovery v5 protocol.
// The caller of this method is responsible
// to send the proper signal.
func (p *PeerPool) startDiscovery(server *p2p.Server) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// already started
	if server.DiscV5 != nil {
		return nil
	}

	ntab, err := StartDiscv5(server)
	if err != nil {
		return err
	}
	server.DiscV5 = ntab

	return nil
}

func (p *PeerPool) stopDiscovery(server *p2p.Server) {
	// do not use defer as we don't want to block sending signal
	p.mu.Lock()

	// already stopped
	if server.DiscV5 == nil {
		p.mu.Unlock()
		return
	}

	for _, t := range p.topicPools {
		t.StopSearch()
	}

	server.DiscV5.Close()
	server.DiscV5 = nil

	p.mu.Unlock()

	signal.SendDiscoveryStopped()
}

// Stop makes sure all background goroutines are stopped.
func (p *PeerPool) Stop() {
	// pool wasn't started
	if p.quit == nil {
		return
	}

	select {
	case <-p.quit:
		// pool was already closed
		return
	default:
		log.Debug("started closing peer pool")
		p.mu.Lock()
		p.running = false
		p.mu.Unlock()
		close(p.quit)
	}

	p.peerEventsSub.Unsubscribe()
	p.wgRuntimeOps.Wait()
	close(p.runtimeOps)
	p.wg.Wait()
}

func (p *PeerPool) handleDiscoveryRuntime(server *p2p.Server) {
	for {
		select {
		case op, ok := <-p.runtimeOps:
			if !ok {
				return
			}
			op(server)
		case <-p.quit:
			log.Debug("stopping DiscV5 due to quit", "server", server.Self())
			p.stopDiscovery(server)
		}
	}
}

func (p *PeerPool) handlePeerEvents(server *p2p.Server, events <-chan *p2p.PeerEvent) {
	for {
		select {
		case <-p.quit:
			return
		case event := <-events:
			switch event.Type {
			case p2p.PeerEventTypeAdd:
				log.Debug("confirm peer added", "ID", event.Peer)
				p.handleAddedPeer(server, event.Peer)
				signal.SendDiscoverySummary(server.PeersInfo())
			case p2p.PeerEventTypeDrop:
				log.Debug("confirm peer dropped", "ID", event.Peer)
				p.handleDroppedPeer(server, event.Peer)
				signal.SendDiscoverySummary(server.PeersInfo())
			}
		}
	}
}

func (p *PeerPool) handleAddedPeer(server *p2p.Server, nodeID discover.NodeID) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	allMaxReached := true
	for _, t := range p.topicPools {
		if !t.ConfirmAdded(server, nodeID) {
			allMaxReached = false
		}
	}

	if p.allowStop && allMaxReached {
		p.runtimeOps <- p.opStopDiscovery(func() {
			log.Info("stopping DiscV5 due to all topics reaching the upper limit", "server", server.Self())
		})
	}
}

func (p *PeerPool) handleDroppedPeer(server *p2p.Server, nodeID discover.NodeID) {
	p.mu.Lock()
	defer p.mu.Unlock()

	anyBelowMin := false
	for _, t := range p.topicPools {
		if t.ConfirmDropped(server, nodeID) {
			anyBelowMin = true
		}
	}

	if p.allowStop && anyBelowMin {
		p.runtimeOps <- p.opStartDiscovery()
	}
}

func (p *PeerPool) addAsyncRuntimeOp(op runtimeOp, timeout <-chan time.Time) {
	p.wgRuntimeOps.Add(1)
	go func() {
		defer p.wgRuntimeOps.Done()

		if timeout == nil {
			p.runtimeOps <- op
			return
		}

		select {
		case <-p.quit:
		case <-timeout:
			p.runtimeOps <- op
		}
	}()
}

func (p *PeerPool) opStartDiscovery() runtimeOp {
	return func(server *p2p.Server) {
		p.wgRuntimeOps.Add(1)
		if err := p.startDiscovery(server); err != nil {
			log.Error("failed to start discovery", "err", err)
			p.addAsyncRuntimeOp(p.opDiscoveryStartFailed(), nil)
		} else {
			p.addAsyncRuntimeOp(p.opDiscoveryStarted(), nil)
		}
	}
}

func (p *PeerPool) opDiscoveryStarted() runtimeOp {
	return func(server *p2p.Server) {
		signal.SendDiscoveryStarted()

		// Stop Discovery v5 if allowed to stop and timeout is defined.
		if p.allowStop && p.discoveryTimeout > 0 {
			p.addAsyncRuntimeOp(
				p.opStopDiscovery(func() {
					log.Info("stopping DiscV5 due to timeout", "server", server.Self())
				}),
				time.After(p.discoveryTimeout),
			)
		}
	}
}

func (p *PeerPool) opDiscoveryStartFailed() runtimeOp {
	return func(_ *p2p.Server) {
		p.addAsyncRuntimeOp(
			p.opStartDiscovery(),
			time.After(defaultDiscoveryRestartTimeout),
		)
	}
}

func (p *PeerPool) opStopDiscovery(logFn func()) runtimeOp {
	return func(server *p2p.Server) {
		if logFn != nil {
			logFn()
		}
		p.stopDiscovery(server)
	}
}
