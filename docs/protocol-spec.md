# Arachne Protocol Specification

## 1. Peer Identity

### 1.1 Key Derivation

```
Operator Key: Ed25519 private key (operator.priv)
Operator PeerID: libp2p PeerID derived from operator.priv public key

Implant Key: Ephemeral Ed25519 (generated on first run)
Implant PeerID: libp2p PeerID from implant public key
```

### 1.2 Implant Certificate
On first execution, the implant generates:
- `implant.ed25519` - ephemeral keypair
- Registration envelope signed by operator key (embedded at build time)

## 2. Topic Structure

All topics use GossipSub via libp2p PubSub.

```
arachne/<operator-peerid>/commands    # Operator -> Implants (read by all implants)
arachne/<operator-peerid>/beacons     # Implants -> Operator (heartbeat & results)
arachne/<operator-peerid>/tasks/<implant-peerid>  # Direct task routing
```

### Topic Authorization
- `commands` topic: messages validated against operator's public key
- `beacons` topic: messages validated against implant's public key
- Implants drop messages not signed by the operator on `commands`
- Operator drops messages not signed by known implants on `beacons`

## 3. Envelope Format

All messages use Protocol Buffers (same approach as Sliver).

```protobuf
message Envelope {
  int64 ID = 1;
  uint32 Type = 2;
  bytes Data = 3;
  bytes Signature = 4;   // Signed by sender's key
  bytes SenderKey = 5;   // Public key of sender
}

message BeaconRegister {
  string ImplantID = 1;
  int64 Interval = 2;
  int64 Jitter = 3;
  Register Register = 4;
}

message Register {
  string Name = 1;
  string Hostname = 2;
  string UUID = 3;
  string Username = 4;
  string UID = 5;
  string GID = 6;
  string OS = 7;
  string Arch = 8;
  int32 PID = 9;
  string Filename = 10;
  string Version = 11;
  int64 PeerID = 12;
}
```

## 4. Session Types

### 4.1 Beacon Mode (Async)
1. Implant subscribes to `commands` topic
2. Implant publishes `BeaconRegister` on `beacons` topic
3. Operator reads beacon, publishes tasks on `tasks/<id>` topic
4. Implant executes tasks, publishes results on `beacons`
5. Implant sleeps for `Interval + random(0, Jitter)`

### 4.2 Interactive Session Mode (Stream)
1. Operator initiates direct libp2p stream to implant
2. Bidirectional encrypted stream for shell/portfwd/socks
3. Uses libp2p stream multiplexing

## 5. Message Types

| Type | ID | Direction | Description |
|---|---|---|---|
| REGISTER | 0 | Implant -> Op | Initial beacon/registration |
| PING | 1 | Bidirectional | Keepalive |
| TASK | 2 | Op -> Implant | Execute command |
| TASK_RESULT | 3 | Implant -> Op | Command output |
| SHELL | 4 | Bidirectional | Interactive shell |
| DOWNLOAD | 5 | Implant -> Op | File exfiltration |
| UPLOAD | 6 | Op -> Implant | File deployment |
| SOCKS | 7 | Bidirectional | SOCKS proxy tunnel |
| PORTFWD | 8 | Bidirectional | Port forwarding |
| SCREENSHOT | 9 | Implant -> Op | Screen capture |
| LS | 10 | Op -> Implant | List directory |
| CD | 11 | Op -> Implant | Change directory |
| EXECUTE | 12 | Op -> Implant | Run command |
| DISCONNECT | 255 | Bidirectional | Clean close |

## 6. Data Exfiltration via IPFS

For large data (files, screenshots, logs):
1. Implant pins data to IPFS and gets a CID
2. Implant sends a small envelope containing the CID + encryption key
3. Operator fetches the data from IPFS using the CID
4. Optionally decrypts with the key sent in-band

This keeps the C2 channel low-bandwidth and hard to detect.
