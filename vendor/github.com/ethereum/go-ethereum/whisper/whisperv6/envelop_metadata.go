package whisperv6

type envelopeSource int

const (
	_ = iota
	// peerSource indicates a source as a regular peer.
	peerSource envelopeSource = iota
	// p2pSource indicates that envelop was received from a trusted peer.
	p2pSource
)

// EnvelopeMeta keeps metadata of received envelopes.
type EnvelopeMeta struct {
	Hash   string
	Topic  TopicType
	Size   uint32
	Source envelopeSource
	IsNew  bool
	Peer   string
}

// SourceString converts source to string.
func (m *EnvelopeMeta) SourceString() string {
	switch m.Source {
	case peerSource:
		return "peer"
	case p2pSource:
		return "p2p"
	default:
		return "unknown"
	}
}

// EnvelopeTracer tracks received envelopes.
type EnvelopeTracer interface {
	Trace(*EnvelopeMeta)
}
