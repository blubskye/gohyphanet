# Hyphanet Node Implementation (Go)

This directory contains a Go implementation of the Hyphanet node-to-node protocol, including the JFK (Just Fast Keying) handshake mechanism used for establishing secure connections between nodes.

## Current Status

✅ **Implemented:**
- Node identity generation (ECDSA P-256)
- ECDH key exchange contexts (P-256)
- JFK Message 1 construction and serialization
- JFK Message 2 parsing
- UDP transport layer
- Basic seed node connection capability

⚠️ **Partially Implemented:**
- Packet encryption/obfuscation layer
- Full JFK handshake (Messages 3 and 4)
- Session key management

❌ **Not Yet Implemented:**
- Complete packet mangler (encryption/decryption)
- Anonymous-initiator auth packets
- Transient key rekeying
- Full peer management
- Routing and request handling
- Node reference parsing and generation

## Architecture

```
node/
├── crypto/
│   ├── identity.go   - Node cryptographic identity (ECDSA)
│   ├── ecdh.go       - ECDH key exchange contexts
│   └── jfk.go        - JFK handshake protocol implementation
├── transport/
│   └── udp.go        - UDP packet transmission
└── node.go           - Main node implementation
```

## How It Works

### JFK Handshake Protocol

The JFK (Just Fast Keying) protocol is used to establish authenticated and encrypted connections between Hyphanet nodes. It consists of 4 messages:

1. **JFK Message 1** (Initiator → Responder)
   - Hashed nonce (Ni')
   - Initiator's ECDH public key (g^i)
   - Initiator's identity hash (for unknown initiators)

2. **JFK Message 2** (Responder → Initiator)
   - Hashed nonce from Message 1 (Ni')
   - Responder's nonce (Nr)
   - Responder's ECDH public key (g^r)
   - ECDSA signature of g^r
   - HMAC authenticator

3. **JFK Message 3** (Initiator → Responder)
   - Encrypted noderef and authenticator

4. **JFK Message 4** (Responder → Initiator)
   - Final authenticator and session setup

Currently, this implementation can **construct and send Message 1** and **parse Message 2**.

### Seed Nodes

Seed nodes are special Hyphanet nodes that help new nodes bootstrap onto the network. They accept connections from unknown initiators (nodes without pre-exchanged keys).

## Testing

### Get Seed Node Addresses

Download the official seed nodes file:
```bash
wget https://raw.githubusercontent.com/hyphanet/java_installer/refs/heads/next/offline/seednodes.fref
```

Extract addresses (they're base64 encoded in `physical.udp` field):
```bash
grep "physical.udp" seednodes.fref | head -1 | cut -d= -f2 | base64 -d
```

### Run Connection Test

```bash
# Build the test tool
go build -o seedconnect ./cmd/seedconnect/

# Try to connect to a seed node
./seedconnect -seed 84.153.137.18 -seed-port 32685 -debug
```

## Known Limitations

1. **Packet Encryption**: The current implementation sends packets without the proper outer obfuscation layer that real Hyphanet nodes expect. This layer is keyed on both nodes' identities.

2. **Incomplete Handshake**: Only JFK Messages 1 and 2 are implemented. A full handshake requires Messages 3 and 4.

3. **No Session Management**: After a successful handshake, nodes need to maintain session keys and encrypted communication channels.

## Next Steps

To make this fully compatible with Hyphanet nodes:

1. **Implement Packet Mangler**
   - Outer encryption using identity-based keys
   - IV and HMAC handling
   - Reference: `fred-next/src/freenet/node/FNPPacketMangler.java`

2. **Complete JFK Handshake**
   - Implement Message 3 construction
   - Implement Message 4 parsing
   - Session key derivation

3. **Node Reference Support**
   - Parse .fref files
   - Generate own noderef
   - Signature verification

4. **Alternative: Java Wrapper**
   - Instead of porting all Java code, create a binary protocol wrapper
   - Call Java implementation via JNI or subprocess
   - Handle handshake through Java, use Go for application logic

## Java Reference

The reference Java implementation is in the `fred-next` repository:
- Handshake: `src/freenet/node/FNPPacketMangler.java`
- Crypto: `src/freenet/node/NodeCrypto.java`
- Seed nodes: `src/freenet/node/SeedClientPeerNode.java`

## License

GNU AGPLv3 - See LICENSE file for details
