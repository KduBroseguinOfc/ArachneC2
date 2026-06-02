# Development Roadmap

## Phase 0: Foundation (Weeks 1-2)
- [x] Project structure and documentation
- [ ] Go module initialization (`go mod init`)
- [ ] libp2p host setup (TCP + WebSocket transports)
- [ ] Ed25519 key generation and PeerID derivation
- [ ] GossipSub topic subscription and publishing
- [ ] Basic Envelope protobuf definition

## Phase 1: Core Protocol (Weeks 3-4)
- [ ] Implant registration and beacon model
- [ ] Operator node: subscribe to beacons, dispatch commands
- [ ] Envelope signing and verification
- [ ] Task execution and result delivery
- [ ] Reconnect / resilience logic

## Phase 2: Implant Capabilities (Weeks 5-8)
- [ ] File system operations (ls, cd, pwd, download, upload)
- [ ] Process enumeration (ps)
- [ ] Command execution (execute, shell)
- [ ] Screenshot capture
- [ ] Cross-platform support (Linux, Windows, macOS)
- [ ] Beacon interval + jitter configuration

## Phase 3: Interactive Sessions (Weeks 9-10)
- [ ] Direct libp2p stream establishment
- [ ] Interactive shell with PTY support
- [ ] Port forwarding
- [ ] SOCKS5 proxy through implant
- [ ] Session multiplexing

## Phase 4: IPFS Integration (Weeks 11-12)
- [ ] IPFS data exfiltration (CID-based file transfer)
- [ ] IPNS for operator key rotation
- [ ] Public pinning service integration (optional)
- [ ] Filecoin storage deals (future)

## Phase 5: Operator Tooling (Weeks 13-14)
- [ ] CLI (Cobra) with all commands
- [ ] TUI console (interactive mode)
- [ ] gRPC local API for GUI clients
- [ ] Multi-operator support
- [ ] Implant generation command

## Phase 6: Advanced Features (Weeks 15-16)
- [ ] Evasion techniques (sandbox detection, AMSI bypass)
- [ ] Process injection and migration
- [ ] Windows token manipulation
- [ ] Pivot through compromised implants
- [ ] COFF/BOF loading (inline execution)

## Phase 7: Hardening (Ongoing)
- [ ] End-to-end encryption review
- [ ] Traffic analysis resistance
- [ ] Protocol fuzzing
- [ ] Go routine leak detection
- [ ] Memory safety review of implant

## Tech Stack

| Component | Technology |
|---|---|
| Language | Go (1.22+) |
| P2P Networking | [go-libp2p](https://github.com/libp2p/go-libp2p) |
| PubSub | [go-libp2p-pubsub](https://github.com/libp2p/go-libp2p-pubsub) |
| Serialization | Protocol Buffers (proto3) |
| DHT | [go-libp2p-kad-dht](https://github.com/libp2p/go-libp2p-kad-dht) |
| Relay | [go-libp2p-circuit](https://github.com/libp2p/go-libp2p-circuit) |
| IPFS | [go-ipfs-api](https://github.com/ipfs/go-ipfs-api) or embedded |
| CLI | [Cobra](https://github.com/spf13/cobra) |
| TUI | [Bubble Tea](https://github.com/charmbracelet/bubbletea) |
| RPC | [gRPC](https://grpc.io/) |
| DB | SQLite (via [modernc.org/sqlite](https://modernc.org/sqlite)) |
