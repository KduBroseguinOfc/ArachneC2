package config

import (
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

type ServerConfig struct {
	OperatorKeyPath string
	ListenAddr      string
	BootstrapPeers  []peer.AddrInfo
	EnableRelay     bool
	EnableMDNS      bool
}

type ImplantConfig struct {
	OperatorPublicKey []byte
	BootstrapPeers    []string
	BeaconInterval    time.Duration
	BeaconJitter      time.Duration
	ReconnectInterval time.Duration
	TransportOrder    []string
	DisableRelay      bool
	EnableAutoNAT     bool
}

var DefaultBootstrapPeers = []string{
	"/dnsaddr/bootstrap.libp2p.io/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN",
	"/dnsaddr/bootstrap.libp2p.io/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa",
	"/dnsaddr/bootstrap.libp2p.io/p2p/QmbLHAnMoJPWSCR5Zhtx6BHJX9KiKNN6tpvbUcqanj75Nb",
}

var DefaultServerConfig = ServerConfig{
	OperatorKeyPath: "operator.key",
	ListenAddr:      "/ip4/0.0.0.0/tcp/0",
	EnableRelay:     true,
	EnableMDNS:      true,
}

var DefaultImplantConfig = ImplantConfig{
	BeaconInterval:    30 * time.Second,
	BeaconJitter:      10 * time.Second,
	ReconnectInterval: 5 * time.Second,
	TransportOrder:    []string{"tcp", "ws", "relay"},
	EnableAutoNAT:     true,
}
