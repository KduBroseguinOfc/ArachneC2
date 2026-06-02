# IPFS & Decentralized Infrastructure Integration

## Why IPFS for C2?

Traditional C2 frameworks rely on centralized infrastructure (VPS, domains, CDNs) that
are single points of failure. IPFS/libp2p provides:

1. **No-cost infrastructure** — use the public IPFS network (no hosting bills)
2. **Censorship resistance** — cannot block a CID, cannot take down a peer easily
3. **NAT traversal** — libp2p relays + AutoNAT + hole-punching eliminate the need for public IPs
4. **Content addressing** — data integrity verified by hash, tamper-proof exfiltration
5. **Persistence** — IPFS pinning services (public/self-hosted) keep data available

## Capabilities Used

### libp2p (Required)

| Feature | Use in Arachne |
|---|---|
| **Peer Identity** | Cryptographic identity (Ed25519) for operator, implants, relays |
| **GossipSub** | PubSub topics for command/beacon messaging |
| **DHT** | Peer discovery, relay discovery, content routing |
| **AutoNAT** | NAT status detection |
| **Circuit Relay** | Relay connections when direct dial fails |
| **Hole Punching** | Direct p2p connections through NAT |
| **Stream Multiplexing** | Multiple concurrent channels per connection |
| **Noise/TLS Security** | Encrypted, authenticated transports |

### IPFS (Optional — recommended)

| Feature | Use in Arachne |
|---|---|
| **Content Addressing** | Exfiltrated files, screenshots, logs identified by CID |
| **Bitswap** | Peer-to-peer block exchange for file retrieval |
| **Pinning** | Keep exfiltrated data available on IPFS |
| **IPNS** | Mutable pointers to operator's latest public key or config |
| **MFS** | Manage file structure for loot organization |

### No-Cost Public Infrastructure

| Resource | How to Use |
|---|---|
| IPFS public bootstrap peers | `/dnsaddr/bootstrap.libp2p.io` — discoverable by default |
| Protocol Labs bootstrap relays | Public relay servers for NAT traversal |
| Public IPFS gateways | `https://ipfs.io/ipfs/<cid>` to fetch exfiltrated data from any browser |
| pinata.cloud / web3.storage | Free tier for pinning exfiltrated IPFS data |
| IPFS desktop / CLI | Run local node or connect to public gateways |

## Data Exfiltration Flow

```
Implant:
  1. Capture screenshot → encode as PNG
  2. Add to local IPFS → get CID: QmX...
  3. Encrypt with one-time key → store alongside
  4. Pin to IPFS (or send to pinning service)
  5. Send tiny envelope on beacons topic:
     { type: SCREENSHOT, data: { cid: "QmX...", key: "..." } }

Operator:
  1. Receive envelope from beacon topic
  2. Fetch data: ipfs get QmX... or https://ipfs.io/ipfs/QmX...
  3. Decrypt with provided key
  4. Store locally
```

## Steganography / Covert Channels

Since all IPFS traffic looks like regular p2p file sharing:
- Encrypted C2 traffic is indistinguishable from normal IPFS block transfers
- Implants can blend in as regular IPFS nodes
- Traffic analysis cannot distinguish C2 from content distribution

## Limitations & Mitigations

| Limitation | Mitigation |
|---|---|
| DHT lookup latency | Keep DHT routing table warm; use direct connections after discovery |
| Public relay bandwidth limits | Self-host relays; use hole-punching for direct connections |
| IPFS public network exposure | Encrypt all data; use ephemeral keys |
| IPFS garbage collection | Pin important data; use pinning services |
| libp2p peer churn | Maintain persistent connections to reliable peers |

## Future: Filecoin Integration

For long-term persistence of exfiltrated data:
- Store data on Filecoin network (decentralized storage marketplace)
- Retrieval deals for fetching data
- Permanent, verifiable storage with economic guarantees
