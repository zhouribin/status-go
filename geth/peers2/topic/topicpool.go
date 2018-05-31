package topic

import (
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/p2p/discv5"
	"github.com/status-im/go-ethereum/common/mclock"
	"github.com/status-im/status-go/geth/params"
	"github.com/status-im/status-go/geth/peers2/cache"
)

const (
	// defaultExpirationPeriod is an amount of time while peer is considered as a connectable
	defaultExpirationPeriod = 60 * time.Minute
)

type peerInfo struct {
	// discoveredTime last time when node was found by v5
	discoveredTime mclock.AbsTime
	// dismissed is true when our node requested a disconnect
	dismissed bool

	node *discv5.Node
}

func newPeerInfo(node *discv5.Node) *peerInfo {
	return &peerInfo{discoveredTime: mclock.Now(), node: node}
}

func (p *peerInfo) hasExpired(expirationTime time.Duration) bool {
	return mclock.Now() > p.discoveredTime+mclock.AbsTime(expirationTime)
}

func (p *peerInfo) updateDiscoveredTimeToNow() {
	p.discoveredTime = mclock.Now()
}

// Pool manages peers for topic.
type Pool struct {
	topic   discv5.Topic
	limits  params.Limits
	running bool
	period  chan time.Duration
	cache   *cache.Cache

	// contains both found and requested to connect but not yet confirmed peers
	pendingPeers map[discv5.NodeID]*peerInfo
	// contains currently connected peers
	connectedPeers map[discv5.NodeID]*peerInfo

	mu     sync.RWMutex
	discWG sync.WaitGroup
	poolWG sync.WaitGroup
	quit   chan struct{}
}

// NewPool returns a new instance of TopicPool.
func NewPool(topic discv5.Topic, limits params.Limits, cache *cache.Cache) *Pool {
	return &Pool{
		topic:          topic,
		limits:         limits,
		period:         make(chan time.Duration, 2),
		cache:          cache,
		pendingPeers:   make(map[discv5.NodeID]*peerInfo),
		connectedPeers: make(map[discv5.NodeID]*peerInfo),
		quit:           make(chan struct{}),
	}
}

// StartSearch creates discv5 queries and runs a loop to consume found peers.
func (t *Pool) StartSearch(server *p2p.Server, period time.Duration) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.running = true
	t.period <- period

	// peers management
	found := make(chan *discv5.Node, 5) // 5 reasonable number for concurrently found nodes
	lookup := make(chan bool, 10)       // sufficiently buffered channel, just prevents blocking because of lookup

	// Get peers from the cache first. Try to get more than the upper limit,
	// in case some of the peers are not available.
	for _, peer := range t.cache.GetPeersRange(t.topic, t.limits.Max*2) {
		log.Debug("adding a peer from cache", "peer", peer)
		found <- peer
	}

	t.poolWG.Add(1)
	go func() {
		t.handleFoundPeers(server, found, lookup)
		t.poolWG.Done()
	}()

	t.discWG.Add(1)
	go func() {
		server.DiscV5.SearchTopic(t.topic, t.period, found, lookup)
		t.discWG.Done()
	}()

	return nil
}

// StopSearch stops the closes stop
func (t *Pool) StopSearch() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.running {
		return
	}

	select {
	case <-t.quit:
		// already closed
		return
	default:
	}

	log.Debug("stoping search", "topic", t.topic)

	close(t.quit)
	// wait for poolWG first because it writes to period channel
	t.poolWG.Wait()
	close(t.period)
	t.discWG.Wait()
}

func (t *Pool) handleFoundPeers(server *p2p.Server, found <-chan *discv5.Node, lookup <-chan bool) {
	selfID := discv5.NodeID(server.Self().ID)
	for {
		select {
		case <-t.quit:
			return
		case node := <-found:
			if node.ID != selfID {
				t.processFoundNode(server, node)
			}
		case <-lookup:
			// noop
		}
	}
}

func (t *Pool) processFoundNode(server *p2p.Server, node *discv5.Node) {
	t.mu.Lock()
	defer t.mu.Unlock()

	log.Debug("peer found", "ID", node.ID, "topic", t.topic)

	// peer is already connected so update only discoveredTime
	if peer, ok := t.connectedPeers[node.ID]; ok {
		peer.updateDiscoveredTimeToNow()
		return
	}

	if peer, ok := t.pendingPeers[node.ID]; ok {
		peer.updateDiscoveredTimeToNow()
	} else {
		t.pendingPeers[peer.node.ID] = newPeerInfo(node)
	}

	// the upper limit is not reached, so let's add this peer
	if !t.isMaxReached() {
		t.addServerPeer(server, t.pendingPeers[node.ID])
	}
}

func (t *Pool) movePeerFromPendingToConnected(nodeID discv5.NodeID) {
	peer, ok := t.pendingPeers[nodeID]
	if !ok {
		return
	}
	delete(t.pendingPeers, nodeID)
	t.connectedPeers[nodeID] = peer
}

// IsSearchRunning returns true if search is running
func (t *Pool) IsSearchRunning() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.running
}

func (t *Pool) isMaxReached() bool {
	return len(t.connectedPeers) >= t.limits.Max
}

// IsMaxReached returns true if we connected with max number of peers.
func (t *Pool) IsMaxReached() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.isMaxReached()
}

func (t *Pool) isBelowMin() bool {
	return len(t.connectedPeers) < t.limits.Min
}

// IsBelowMin returns true if current number of peers is below min limit.
func (t *Pool) IsBelowMin() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.isBelowMin()
}

// ConfirmAdded is called when a peer has been added to the p2p server successfully.
// It returns true, if the upper limit was reached.
func (t *Pool) ConfirmAdded(server *p2p.Server, nodeID discover.NodeID) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	discV5NodeID := discv5.NodeID(nodeID)

	// It's an inbound connection.
	peer, ok := t.pendingPeers[discV5NodeID]
	if !ok {
		return t.isMaxReached()
	}

	// Established connection means that the node is a viable candidate
	// for a connection and can be cached.
	if err := t.cache.AddPeer(peer.node, t.topic); err != nil {
		log.Error("failed to persist a peer", "error", err)
	}

	t.movePeerFromPendingToConnected(discV5NodeID)

	// If the number of connected peers exceeds the upper limit,
	// ask to drop the peer.
	if len(t.connectedPeers) > t.limits.Max {
		log.Debug("max limit is reached drop the peer", "ID", nodeID, "topic", t.topic)
		peer.dismissed = true
		t.removeServerPeer(server, peer)
	}

	return t.isMaxReached()
}

// ConfirmDropped is called when a peer has been removed from the p2p server.
// It returns true, if the number of connected peers dropped below the lower limit.
func (t *Pool) ConfirmDropped(server *p2p.Server, nodeID discover.NodeID) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	discV5NodeID := discv5.NodeID(nodeID)

	// either inbound or connected from another topic
	peer, exist := t.connectedPeers[discV5NodeID]
	if !exist {
		return t.isBelowMin()
	}

	log.Debug("confirm dropped", "ID", nodeID, "dismissed", peer.dismissed)

	delete(t.connectedPeers, discV5NodeID)

	// Peer was removed by us because exceeded the limit.
	// Add it back to the pool as it can be useful in the future.
	if peer.dismissed {
		// reset dismissed
		peer.dismissed = false
		t.pendingPeers[discV5NodeID] = peer
		return t.isBelowMin()
	}

	// If there was a network error, this event will be received
	// but the peer won't be removed from the static nodes set.
	// That's why we need to call `removeServerPeer` manually.
	t.removeServerPeer(server, peer)

	if err := t.cache.RemovePeer(discV5NodeID, t.topic); err != nil {
		log.Error("failed to remove peer from cache", "error", err)
	}

	// We may haved dropped below the upper limit. Try to compensate it.
	if !t.isMaxReached() {
		t.addPeerFromTable(server)
	}

	return t.isBelowMin()
}

// addPeerFromTable checks if there is a valid peer in local table and adds it to a server.
func (t *Pool) addPeerFromTable(server *p2p.Server) {
	newConnLimit := (t.limits.Max - len(t.connectedPeers)) * 2
	added := 0

	for key, peer := range t.pendingPeers {
		if peer.hasExpired(defaultExpirationPeriod) {
			delete(t.pendingPeers, key)
		} else {
			t.addServerPeer(server, peer)
			added++
		}

		if added == newConnLimit {
			break
		}
	}
}

func (t *Pool) addServerPeer(server *p2p.Server, peer *peerInfo) {
	server.AddPeer(discover.NewNode(
		discover.NodeID(peer.node.ID),
		peer.node.IP,
		peer.node.UDP,
		peer.node.TCP,
	))
}

func (t *Pool) removeServerPeer(server *p2p.Server, peer *peerInfo) {
	server.RemovePeer(discover.NewNode(
		discover.NodeID(peer.node.ID),
		peer.node.IP,
		peer.node.UDP,
		peer.node.TCP,
	))
}
