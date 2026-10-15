# METAL GEAR SOLID: Peace Walker

Nextendo NPLN server for METAL GEAR SOLID: Peace Walker - Master Collection Version.

- Title ID `0100C6F01C4F8000`
- NPLN tenant `t-4f53195c-lp1`

Both are the defaults in `token_jwt.go` and can be overridden with `NPLN_APP_ID` and `NPLN_TENANT_ID`.

It implements NPLN authentication, matchmaking, Gamesync, messaging, friends, UGC, NNCS NAT-check, STUN and TURN, and is deployed on Nextendo production.

## Protocol notes

- **Signalling payload.** Peace Walker sends an `mp` map (SDP offer plus DTLS certificate), written into the recipient's document (`gamesync_store.go`).
- **Lobby capacity from the config name.** Matchmaking configurations such as `Mmc_CoOps2` carry their capacity as a trailing number, which is parsed out rather than hardcoded (`matchmaking.go`).
- **JoinGameSession membership.** The join response returns the joining member rather than every member, matching a captured Nintendo response; returning all of them broke the client (`session_service.go`).

## TURN, and why it is required

The NPLN mesh setup stops in `AttachMeshJob::WaitSetupRelayAddress` when the client receives STUN configuration but no TURN server. This implementation advertises an authenticated RFC 8656 UDP TURN endpoint and runs the matching relay, after which `CreateMesh` and `JoinMesh` complete.

## Friend discovery

Friend activity is discovered through `QueryGameSessions` using the `FriendSearch` configuration, a friend UID and a `MatchingKey` property. The server validates the friendship, applies the requested session filters and returns only visible active rooms. Friend session pools stay linked to the selected base room through `FriendGameSessionId`.

## Build

Use Go 1.26.4 or a compatible newer release:

```sh
go test ./...
go build -o peacewalker-server .
```

Copy the settings from `example.env` into your environment. Provide your own Nextendo CA certificate and key, server certificate path, account service secrets and TURN password. The ES256 NPLN key is generated at `NPLN_JWT_KEY` when it does not already exist.

The successful local layout used:

- NPLN TLS/gRPC on TCP 443;
- STUN on `127.0.0.1:3478/udp`;
- TURN on `127.0.0.1:3479/udp`;
- NNCS on `127.0.0.1` and `127.0.0.2`;
- the Nextendo account service on `127.0.0.1:18080`.

Route the NPLN hostnames used by the client to this server through the normal Nextendo DNS or redirection layer. The defaults are for local development and are not an internet deployment configuration.

## Advertised endpoints and shared-server ports

`AllocateIceServerSet` uses `NPLN_STUN_HOST` and `NPLN_STUN_PORT` for the client-facing UDP STUN endpoint (defaults: `127.0.0.1`, `3478`). `NPLN_STUN_LISTEN` independently controls the local bind address. Set both when moving STUN to another port; a wildcard bind address such as `0.0.0.0` must not be advertised to clients.

For example, if UDP 3478 is already occupied by another game's coturn, choose available ports:

```sh
NPLN_STUN_LISTEN=0.0.0.0:3480
NPLN_STUN_HOST=<client-reachable-server-address>
NPLN_STUN_PORT=3480
NPLN_TURN_LISTEN=0.0.0.0:3481
NPLN_TURN_HOST=<client-reachable-server-address>
NPLN_TURN_PORT=3481
```

Replace the placeholders before starting the server. Allow the advertised UDP ports through the firewall and any port forwarding. TURN also allocates separate UDP relay sockets on OS-assigned ports; allowing only its listener port is insufficient for relayed traffic. The current embedded TURN implementation uses `NPLN_TURN_RELAY_IP` both as the advertised relay IPv4 address and the local relay bind address, so that IP must be assigned to the server and reachable by clients. This example alone does not configure a TURN deployment behind NAT.

`ListLatencyMeasurementServers` uses `NPLN_LATENCY_HOST` and `NPLN_LATENCY_PORT` (default port: `443`). When the latency host is unset, it falls back to `NPLN_GAMESESSION_HOST`, then `127.0.0.1`. Set it to the reachable latency endpoint along with the other deployment addresses in `example.env`; changing STUN does not update GameSession or NNCS addresses. Existing same-PC defaults are preserved.

## Client patch

NPLN titles pin Nintendo's certificate and reject the peer name presented by a local server, so the client needs a certificate patch to reach this server.

That patch is **built into the emulators** and needs nothing from this repository: citron-nextendo and Ryujinx-Nextendo both carry it in their NPLN patch tables, gated on the title ID and the build ID.

`client-patches/generate_ips32.py` can generate a standalone IPS32 patch, optionally verifying the original instructions against a legally obtained decompressed `main` image. The `.patch` file beside it targets a different title and does not apply here. The repository does not include game files or a client binary.

## TURN behavior

`AllocateIceServerSet` returns one UDP TURN server using `NPLN_TURN_HOST`, `NPLN_TURN_PORT`, `NPLN_TURN_USERNAME` and `NPLN_TURN_PASSWORD`. `turn.go` starts the corresponding Pion TURN v4 listener using `NPLN_TURN_LISTEN`, `NPLN_TURN_RELAY_IP` and `NPLN_TURN_REALM`.

`turn_test.go` authenticates a real TURN client, creates an allocation and verifies a UDP request/reply through the relay. It also checks the protobuf advertisement.

## Current limits

The captured `docs/__mt/nat_traversal` monitoring document is accepted only when its local and remote users are active members of the same GameSession. `FriendSearch` is the only implemented named GameSession search configuration; unknown configurations remain unsupported until observed.

Keep request traces private because they can contain account identifiers and network addresses. No private keys, account data, game files, emulator binaries, logs or packet captures are included.

Based on [Nextendo Network](https://github.com/NextendoNetwork)'s service layout and the public NPLN protocol types in [Kinnay's NintendoClients](https://github.com/kinnay/NintendoClients). Original code remains under its license.
