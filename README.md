# cosmos-exporter
1. `make install`
2. `cosmos-expoter start --home /path/to/config/file/config.yaml`


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