package hostrunner

// Flag names.
const (
	configName                   = "config"
	nodeIDName                   = "id"
	isGenesisName                = "isGenesis"
	nodeTypeName                 = "nodeType"
	gossipRoundNanosName         = "gossipRoundNanos"
	clientRPCPortHTTPName        = "clientRPCPortHttp"
	clientRPCPortWSName          = "clientRPCPortWs"
	clientRPCHostName            = "clientRPCHost"
	enclaveRPCAddressName        = "enclaveRPCAddress"
	p2pBindAddressName           = "p2pBindAddress"
	p2pPublicAddressName         = "p2pPublicAddress"
	l1NodeHostName               = "l1NodeHost"
	l1NodePortName               = "l1NodePort"
	clientRPCTimeoutSecsName     = "clientRPCTimeoutSecs"
	enclaveRPCTimeoutSecsName    = "enclaveRPCTimeoutSecs"
	l1RPCTimeoutSecsName         = "l1RPCTimeoutSecs"
	p2pConnectionTimeoutSecsName = "p2pConnectionTimeoutSecs"
	rollupContractAddrName       = "rollupContractAddress"
	logLevelName                 = "logLevel"
	logPathName                  = "logPath"
	privateKeyName               = "privateKey"
	pkAddressName                = "pkAddress"
	l1ChainIDName                = "l1ChainID"
	obscuroChainIDName           = "obscuroChainID"
	profilerEnabledName          = "profilerEnabled"
)

// Returns a map of the flag usages.
// While we could just use constants instead of a map, this approach allows us to test that all the expected flags are defined.
func getFlagUsageMap() map[string]string {
	return map[string]string{
		configName:                   "The path to the host's config file. Overrides all other flags",
		nodeIDName:                   "The 20 bytes of the host's address",
		isGenesisName:                "Whether the host is the first host to join the network",
		nodeTypeName:                 "The node's type (e.g. aggregator, validator)",
		gossipRoundNanosName:         "The duration of the gossip round",
		clientRPCPortHTTPName:        "The port on which to listen for client application RPC requests over HTTP",
		clientRPCPortWSName:          "The port on which to listen for client application RPC requests over websockets",
		clientRPCHostName:            "The host on which to handle client application RPC requests",
		enclaveRPCAddressName:        "The address to use to connect to the Obscuro enclave service",
		p2pBindAddressName:           "The address where the p2p server is bound to. Defaults to 0.0.0.0:10000",
		p2pPublicAddressName:         "The P2P address where the other servers should connect to. Defaults to 127.0.0.1:10000",
		l1NodeHostName:               "The network host on which to connect to the Ethereum client",
		l1NodePortName:               "The port on which to connect to the Ethereum client",
		clientRPCTimeoutSecsName:     "The timeout for client <-> host RPC communication",
		enclaveRPCTimeoutSecsName:    "The timeout for host <-> enclave RPC communication",
		l1RPCTimeoutSecsName:         "The timeout for connecting to, and communicating with, the Ethereum client",
		p2pConnectionTimeoutSecsName: "The timeout for host <-> host P2P messaging",
		rollupContractAddrName:       "The management contract address on the L1",
		logLevelName:                 "The verbosity level of logs. (Defaults to Info)",
		logPathName:                  "The path to use for the host's log file",
		privateKeyName:               "The private key for the L1 host account",
		pkAddressName:                "The address of the L1 host account",
		l1ChainIDName:                "An integer representing the unique chain id of the Ethereum chain used as an L1 (default 1337)",
		obscuroChainIDName:           "An integer representing the unique chain id of the Obscuro chain (default 777)",
		profilerEnabledName:          "Runs a profiler instance (Defaults to false)",
	}
}
