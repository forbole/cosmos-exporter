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