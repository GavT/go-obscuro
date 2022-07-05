package hostrunner

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/obscuronet/obscuro-playground/go/common/log"

	"github.com/obscuronet/obscuro-playground/go/ethadapter"

	"github.com/obscuronet/obscuro-playground/go/config"

	"github.com/obscuronet/obscuro-playground/go/wallet"

	"github.com/obscuronet/obscuro-playground/go/ethadapter/mgmtcontractlib"
	"github.com/obscuronet/obscuro-playground/go/host"
	"github.com/obscuronet/obscuro-playground/go/host/p2p"
)

// RunHost runs an Obscuro host as a standalone process.
func RunHost(config config.HostConfig) {
	mgmtContractLib := mgmtcontractlib.NewMgmtContractLib(&config.RollupContractAddress)

	fmt.Println("Connecting to L1 network...")
	l1Client, err := ethadapter.NewEthClientFromConfig(config)
	if err != nil {
		log.Panic("could not create Ethereum client. Cause: %s", err)
	}

	ethWallet := wallet.NewInMemoryWalletFromConfig(config)
	nonce, err := l1Client.Nonce(ethWallet.Address())
	if err != nil {
		log.Panic("could not retrieve Ethereum account nonce. Cause: %s", err)
	}
	ethWallet.SetNonce(nonce)

	enclaveClient := host.NewEnclaveRPCClient(config)
	aggP2P := p2p.NewSocketP2PLayer(config)
	agg := host.NewHost(config, nil, aggP2P, l1Client, enclaveClient, ethWallet, mgmtContractLib)

	fmt.Println("Starting Obscuro host...")
	log.Info("Starting Obscuro host...")
	agg.Start()

	handleInterrupt(agg)
}

// SetLogs sets the log file.
func SetLogs(logPath string) {
	logFile, err := os.Create(logPath)
	if err != nil {
		panic(fmt.Sprintf("could not create log file. Cause: %s", err))
	}
	log.OutputToFile(logFile)
}

// Shuts down the Obscuro host when an interrupt is received.
func handleInterrupt(host *host.Node) {
	interruptChannel := make(chan os.Signal, 1)
	signal.Notify(interruptChannel, os.Interrupt, syscall.SIGTERM)
	<-interruptChannel
	host.Stop()
	fmt.Println("Obscuro host stopping...")
	os.Exit(1)
}