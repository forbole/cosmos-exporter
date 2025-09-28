# cosmos-exporter
1. `make install`
2. `cosmos-expoter start --home /path/to/config/file/config.yaml`

## Compatibility
This version has been upgraded to support:
- Cosmos SDK v0.50.x
- CometBFT v0.38+

The exporter should still work with older chains, but is optimized for newer versions.

### Backward Compatibility
The exporter maintains backward compatibility with:
- Older Cosmos SDK versions (v0.45.x and earlier)
- Tendermint nodes (pre-CometBFT)

This allows for a smooth upgrade path, as the same exporter can be used across different chain versions in your infrastructure.

## Runtime Resilience & Failover
The exporter now supports automatic runtime failover across multiple gRPC and RPC endpoints.

### Endpoint Sources
1. Config file (`node.grpc`, `node.rpc`)
2. Chain Registry (when `--chain-name <name>` is supplied and a local chain-registry clone is present)

All discovered endpoints are deduplicated; gRPC and RPC each maintain their own candidate list.

### Rotation Logic
The exporter tracks consecutive errors for gRPC and RPC:
- On every failed gRPC query (e.g. governance proposals, votes), an error streak counter increments.
- When the streak reaches the configured threshold, the exporter rotates to the next gRPC endpoint, redials, and resets the streak.
- RPC rotation (currently used for chain ID refresh logic) follows the same pattern.

### Environment Variables (Threshold Overrides)
| Variable | Description | Default |
|----------|-------------|---------|
| `COSMOS_EXPORTER_MAX_GRPC_ERRORS` | Consecutive gRPC errors before rotating | `3` |
| `COSMOS_EXPORTER_MAX_RPC_ERRORS`  | Consecutive RPC errors before rotating  | `3` |

Set to `1` for quick testing (forces rotation on first error).

### CLI Flags
| Flag | Description | Default |
|------|-------------|---------|
| `--chain-name` | Enable chain-registry endpoint discovery | (empty) |
| `--chain-registry-path` | Path to local chain-registry clone | `./chain-registry` |
| `--listen-address` | Override listen port/address (e.g. `:26647`) | value from config `port` |
| `--scrape-interval` | Scrape interval (`30s`, `1m`, `5m`, etc.) | `10m` |

### Example: Fast Rotation Test
```bash
export COSMOS_EXPORTER_MAX_GRPC_ERRORS=1
export COSMOS_EXPORTER_MAX_RPC_ERRORS=2
./build/cosmos_exporter start \
  --home ~/.cosmos-exporter \
  --chain-name xpla \
  --chain-registry-path ./chain-registry \
  --listen-address :26647 \
  --scrape-interval 30s
```

Expected logs when an endpoint is flaky:
```
[gRPC] error streak=1/1 err=...
[gRPC] rotating endpoint error_streak=1 old=grpc.old.endpoint new=grpc.new.endpoint
```

If only one endpoint exists, rotation will not occur (streak logs appear but no switch). Add a second endpoint via config or chain registry to test.

### Operational Tips
- Use a longer interval (e.g. `--scrape-interval 5m`) in production to reduce load.
- Keep thresholds >1 in production to avoid rotating on transient blips.
- Monitor logs (or extend with Prometheus metrics) to observe rotation behavior.
- You can pin a single endpoint by setting thresholds very high (e.g. `COSMOS_EXPORTER_MAX_GRPC_ERRORS=999999`).

### Future Improvements (optional)
Potential follow-ups you can implement or request:
- Prometheus counters for rotation events.
- Readiness/health endpoint.
- CLI flags for thresholds instead of env vars.
- Extending rotation triggers to all collectors (currently governance-focused for gRPC error streaks).

# Config file template
```yaml
delegator_addresses: 
  - "delegator_address"
validator_address: "validator_address"
port: ":9092"
denom_metadata:
 display_denom: "atom"
 base_denom: "uatom"
 exponent: 6
node:
 rpc: "http://localhost:26657"
 grpc: "localhost:9090"
 secure: false
```

## Chain Registry Submodule
This repository uses the official `cosmos/chain-registry` as a Git submodule (directory: `chain-registry`).

### First Clone
```bash
git clone <your-fork-or-repo>
cd cosmos-exporter
git submodule update --init --recursive
```

### Update to Latest Registry
```bash
git submodule update --remote --merge chain-registry
git commit -am "Update chain-registry submodule"
```

### Shallow Clone (already configured)
The submodule was added with `--depth 1` to save time/space. To deepen later:
```bash
cd chain-registry
git fetch --unshallow || true
```

### Sparse Checkout (optional)
If you only need a few chains (example: xpla, cosmoshub):
```bash
cd chain-registry
git sparse-checkout init --cone
git sparse-checkout set xpla cosmoshub
```

### Removing the Submodule (if ever needed)
```bash
git submodule deinit -f chain-registry
git rm -f chain-registry
rm -rf .git/modules/chain-registry
git commit -m "Remove chain-registry submodule"
```

## Systemd Service Example
Create `/etc/systemd/system/cosmos-exporter-xpla.service`:
```ini
[Unit]
Description=Cosmos Exporter (XPLA)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=/opt/cosmos-exporter
ExecStart=/opt/cosmos-exporter/build/cosmos_exporter start \\
  --home /opt/cosmos-exporter/.xpla-exporter \\
  --chain-name xpla \\
  --chain-registry-path /opt/cosmos-exporter/chain-registry \\
  --listen-address :26647 \\
  --scrape-interval 1m
Environment=COSMOS_EXPORTER_MAX_GRPC_ERRORS=3
Environment=COSMOS_EXPORTER_MAX_RPC_ERRORS=3
Restart=on-failure
RestartSec=5
User=cosmos
Group=cosmos

[Install]
WantedBy=multi-user.target
```

Then:
```bash
sudo systemctl daemon-reload
sudo systemctl enable --now cosmos-exporter-xpla
sudo systemctl status cosmos-exporter-xpla --no-pager
```