package transport

import (
	"context"
	"fmt"
	"sync"

	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multiaddr"
)

const (
	ArachneProtocolID  protocol.ID = "/arachne/1.0.0"
	CommandTopicPrefix  string     = "arachne/cmd/"
	BeaconTopicPrefix   string     = "arachne/beacon/"
	TaskTopicPrefix     string     = "arachne/task/"
)

type NodeConfig struct {
	ListenAddr     string
	BootstrapPeers []peer.AddrInfo
	EnableRelay    bool
	EnableMDNS     bool
}

type Node struct {
	Host   host.Host
	PubSub *pubsub.PubSub
	config NodeConfig
	topics map[string]*pubsub.Topic
	subs   map[string]*pubsub.Subscription
	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
}

func NewNode(ctx context.Context, cfg NodeConfig, opts ...libp2p.Option) (*Node, error) {
	ctx, cancel := context.WithCancel(ctx)

	baseOpts := []libp2p.Option{
		libp2p.ListenAddrStrings(cfg.ListenAddr),
		libp2p.NATPortMap(),
	}

	if cfg.EnableRelay {
		baseOpts = append(baseOpts, libp2p.EnableRelay())
	}

	baseOpts = append(baseOpts, opts...)

	h, err := libp2p.New(baseOpts...)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create libp2p host: %w", err)
	}

	ps, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create pubsub: %w", err)
	}

	return &Node{
		Host:   h,
		PubSub: ps,
		config: cfg,
		topics: make(map[string]*pubsub.Topic),
		subs:   make(map[string]*pubsub.Subscription),
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

func (n *Node) StartDiscovery() error {
	for _, pi := range n.config.BootstrapPeers {
		if err := n.Host.Connect(n.ctx, pi); err != nil {
			continue
		}
	}
	return nil
}

func (n *Node) JoinTopic(topic string) (*pubsub.Topic, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if t, ok := n.topics[topic]; ok {
		return t, nil
	}

	t, err := n.PubSub.Join(topic)
	if err != nil {
		return nil, fmt.Errorf("join topic %s: %w", topic, err)
	}
	n.topics[topic] = t
	return t, nil
}

func (n *Node) Subscribe(topic string) (*pubsub.Subscription, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if s, ok := n.subs[topic]; ok {
		return s, nil
	}

	t, err := n.JoinTopic(topic)
	if err != nil {
		return nil, err
	}

	sub, err := t.Subscribe()
	if err != nil {
		return nil, fmt.Errorf("subscribe %s: %w", topic, err)
	}
	n.subs[topic] = sub
	return sub, nil
}

func (n *Node) Publish(ctx context.Context, topic string, data []byte) error {
	t, err := n.JoinTopic(topic)
	if err != nil {
		return err
	}
	return t.Publish(ctx, data)
}

func (n *Node) ConnectToPeer(ctx context.Context, pi peer.AddrInfo) error {
	return n.Host.Connect(ctx, pi)
}

func (n *Node) SetStreamHandler(pid protocol.ID, handler network.StreamHandler) {
	n.Host.SetStreamHandler(pid, handler)
}

func (n *Node) NewStream(ctx context.Context, p peer.ID, pid protocol.ID) (network.Stream, error) {
	return n.Host.NewStream(ctx, p, pid)
}

func (n *Node) Close() error {
	n.cancel()
	return n.Host.Close()
}

func (n *Node) Addrs() []multiaddr.Multiaddr {
	return n.Host.Addrs()
}

func (n *Node) ID() peer.ID {
	return n.Host.ID()
}

func (n *Node) AddrsWithID() []multiaddr.Multiaddr {
	var addrs []multiaddr.Multiaddr
	for _, a := range n.Host.Addrs() {
		addrs = append(addrs, a.Encapsulate(multiaddr.StringCast("/p2p/" + n.Host.ID().String())))
	}
	return addrs
}
