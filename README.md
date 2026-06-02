# Arachne C2

A decentralized Command & Control framework built on **libp2p** (the peer-to-peer networking
stack behind IPFS). No servers, no domains, no IPs to block — just cryptographic identities
communicating over the global p2p network.

Inspired by [Sliver](https://github.com/BishopFox/sliver) (BishopFox's adversary emulation
framework), but redesigned for decentralized infrastructure.

## Key Idea

Instead of running a C2 server on a VPS that can be taken down, Arachne uses **GossipSub
(PubSub)** and **direct libp2p streams** over the IPFS peer-to-peer network. Your implant
fleet and operator are all equal peers in the network — no central point of failure.

## Features (Planned)

- No-cost infrastructure — uses public libp2p network
- Censorship-resistant — no IPs or domains to block
- Beacon mode (async) and session mode (interactive)
- IPFS-based data exfiltration (CID-addressed files)
- Interactive shell, port forwarding, SOCKS5 proxy
- Cross-platform implants (Windows, Linux, macOS)
- Multi-operator support
- Protocol Buffers message format
- Ed25519 cryptographic identities

## Project Structure

```
arachne/
├── docs/                  # Design documentation
│   ├── architecture.md    # System architecture
│   ├── protocol-spec.md   # Wire protocol specification
│   ├── implant-design.md  # Implant architecture
│   ├── server-operator-design.md  # Operator node design
│   ├── ipfs-integration.md        # IPFS/libp2p usage
│   └── development-roadmap.md     # Build plan
├── server/                # Operator node (the "server")
├── implant/               # Implant agent code
├── client/                # CLI / TUI client
├── protobuf/              # Protocol Buffers definitions
└── pkg/                   # Shared libraries
    ├── cryptography/      # Crypto primitives
    ├── transport/         # libp2p transport helpers
    ├── config/            # Shared config types
    └── util/              # Utilities
```

## Getting Started

_Coming soon — see the [development roadmap](docs/development-roadmap.md)._

## License

GPLv3 (same as Sliver)
