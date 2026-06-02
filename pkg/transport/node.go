package transport

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"

	"github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	"github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	"github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/client"

	"github.com/multiformats/go-multiaddr"
)

const (
	ArachneProtocolID   protocol.ID = "/arachne/1.0.0"
	CommandTopicPrefix  string      = "arachne/cmd/"
	BeaconTopicPrefix   string      = "arachne/beacon/"
	TaskTopicPrefix     string      = "arachne/task/"
)

type NodeConfig struct {
	ListenAddr     string
	BootstrapPeers []peer.AddrInfo
	EnableRelay    bool
	EnableMDNS     bool
	RelayV2        bool
}

type Node struct {
	Host    host.Host
	PubSub  *pubsub.PubSub
	DHT     *routing.RoutingDiscovery
	config  NodeConfig
	topics  map[string]*pubsub.Topic
	subs    map[string]*pubsub.Subscription
	mu      sync.RWMutex
	ctx     context.Context
	cancel  context.CancelFunc
}

func NewNode(ctx context.Context, cfg NodeConfig, opts ...libp2p.Option) (*Node, error) {
	ctx, cancel := context.WithCancel(ctx)

	baseOpts := []libp2p.Option{
		libp2p.ListenAddrStrings(cfg.ListenAddr),
		libp2p.EnableAutoNATv2(),
		libp2p.NATPortMap(),
		libp2p.EnableHolePunching(),
	}

	if cfg.EnableRelay {
		baseOpts = append(baseOpts,
			libp2p.EnableRelay(),
		)
	}

	baseOpts = append(baseOpts, opts...)

	h, err := libp2p.New(baseOpts...)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create libp2p host: %w", err)
	}

	ps, err := pubsub.NewGossipSub(ctx, h,
		pubsub.WithMessageSignaturePolicy(pubsub.StrictNoSign),
	)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create pubsub: %w", err)
	}

	node := &Node{
		Host:   h,
		PubSub: ps,
		config: cfg,
		topics: make(map[string]*pubsub.Topic),
		subs:   make(map[string]*pubsub.Subscription),
		ctx:    ctx,
		cancel: cancel,
	}

	if cfg.RelayV2 {
		_, err := relay.New(h)
		if err != nil {
			return nil, fmt.Errorf("enable relay v2: %w", err)
		}
	}

	return node, nil
}

func (n *Node) StartDiscovery() error {
	for _, pi := range n.config.BootstrapPeers {
		if err := n.Host.Connect(n.ctx, pi); err != nil {
			continue
		}
	}

	if n.config.EnableMDNS {
		mdnsSvc := mdns.NewMdnsService(n.Host, "", &mdnsNotifee{h: n.Host})
		if err := mdnsSvc.Start(); err != nil {
			return fmt.Errorf("start mdns: %w", err)
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

func (n *Node) Peerstore() peerstore {
	return n.Host.Peerstore()
}

type peerstore interface {
	AddAddr(peer.ID, multiaddr.Multiaddr, time.Duration)
}

type mdnsNotifee struct {
	h host.Host
}

func (m *mdnsNotifee) HandlePeerFound(pi peer.AddrInfo) {
	if pi.ID == m.h.ID() {
		return
	}
	m.h.Peerstore().AddAddr(pi.ID, pi.Addrs[0], time.Hour)
}

func ConnectToRelay(ctx context.Context, h host.Host, relayAddr string) error {
	maddr, err := multiaddr.NewMultiaddr(relayAddr)
	if err != nil {
		return err
	}
	relayInfo, err := peer.AddrInfoFromP2pAddr(maddr)
	if err != nil {
		return err
	}
	if err := h.Connect(ctx, *relayInfo); err != nil {
		return err
	}
	return nil
}

func DialThroughRelay(ctx context.Context, h host.Host, relayID peer.ID, targetID peer.ID, targetAddr multiaddr.Multiaddr) (network.Stream, error) {
	_, err := client.Reserve(ctx, h, relayID)
	if err != nil {
		return nil, fmt.Errorf("reserve relay: %w", err)
	}
	addr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/p2p/%s/p2p-circuit/p2p/%s", relayID, targetID))
	if err != nil {
		return nil, err
	}

	pi := peer.AddrInfo{ID: targetID, Addrs: []multiaddr.Multiaddr{addr}}
	return h.NewStream(ctx, pi.ID, ArachneProtocolID)
}
