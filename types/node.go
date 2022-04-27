package types

type Node struct {
	RPC      string `mapstructure:"rpc"`
	GRPC     string `mapstructure:"grpc"`
	IsSecure bool   `mapstructure:"secure"`
}

func NewNode(
	rpc string, grpc string, isSecure bool,
) *Node {
	return &Node{
		RPC:      rpc,
		GRPC:     grpc,
		IsSecure: isSecure,
	}
}

func DefaultNodeConfig() *Node {
	return NewNode("localhost:26657", "localhost:9090", false)
}
