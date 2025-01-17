package host

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sync/atomic"
	"time"

	hostcommon "github.com/obscuronet/go-obscuro/go/common/host"

	"github.com/obscuronet/go-obscuro/go/common/retry"

	gethlog "github.com/ethereum/go-ethereum/log"

	"github.com/obscuronet/go-obscuro/go/host/events"

	"github.com/obscuronet/go-obscuro/go/host/rpc/clientapi"

	"github.com/obscuronet/go-obscuro/go/host/db"
	"github.com/obscuronet/go-obscuro/go/host/rpc/clientrpc"

	"github.com/ethereum/go-ethereum/rpc"

	"github.com/obscuronet/go-obscuro/go/common/profiler"

	"github.com/obscuronet/go-obscuro/go/common/log"

	"github.com/ethereum/go-ethereum"
	gethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/naoina/toml"
	"github.com/obscuronet/go-obscuro/go/common"
	"github.com/obscuronet/go-obscuro/go/config"
	"github.com/obscuronet/go-obscuro/go/ethadapter"
	"github.com/obscuronet/go-obscuro/go/ethadapter/mgmtcontractlib"
	"github.com/obscuronet/go-obscuro/go/wallet"
)

const (
	APIVersion1             = "1.0"
	APINamespaceObscuro     = "obscuro"
	APINamespaceEth         = "eth"
	apiNamespaceObscuroScan = "obscuroscan"
	apiNamespaceNetwork     = "net"
	apiNamespaceTest        = "test"

	// Attempts to broadcast the rollup transaction to the L1. Worst-case, equates to 7 seconds, plus time per request.
	l1TxTriesRollup = 3
	// Attempts to send secret initialisation, request or response transactions to the L1. Worst-case, equates to 63 seconds, plus time per request.
	l1TxTriesSecret = 7

	maxWaitForL1Receipt       = 100 * time.Second
	retryIntervalForL1Receipt = 10 * time.Second
)

// Implementation of host.Host.
type host struct {
	config      config.HostConfig
	shortID     uint64
	isSequencer bool

	p2p           hostcommon.P2P       // For communication with other Obscuro nodes
	ethClient     ethadapter.EthClient // For communication with the L1 node
	enclaveClient common.Enclave       // For communication with the enclave
	rpcServer     clientrpc.Server     // For communication with Obscuro client applications

	stats hostcommon.StatsCollector

	// control the host lifecycle
	exitHostCh            chan bool
	stopHostInterrupt     *int32
	bootstrappingComplete *int32 // Marks when the host is done bootstrapping

	blockRPCCh chan blockAndParent        // The channel that new blocks from the L1 node are sent to
	forkRPCCh  chan []common.EncodedBlock // The channel that new forks from the L1 node are sent to
	txP2PCh    chan common.EncryptedTx    // The channel that new transactions from peers are sent to

	hostDB *db.DB // Stores the host's publicly-available data

	mgmtContractLib mgmtcontractlib.MgmtContractLib // Library to handle Management Contract lib operations
	ethWallet       wallet.Wallet                   // Wallet used to issue ethereum transactions
	logEventManager events.LogEventManager

	logger gethlog.Logger
}

func NewHost(config config.HostConfig, stats hostcommon.StatsCollector, p2p hostcommon.P2P, ethClient ethadapter.EthClient, enclaveClient common.Enclave, ethWallet wallet.Wallet, mgmtContractLib mgmtcontractlib.MgmtContractLib, logger gethlog.Logger) hostcommon.MockHost {
	host := &host{
		// config
		config:      config,
		shortID:     common.ShortAddress(config.ID),
		isSequencer: config.IsGenesis && config.NodeType == common.Aggregator,

		// Communication layers.
		p2p:           p2p,
		ethClient:     ethClient,
		enclaveClient: enclaveClient,

		// statistics and metrics
		stats: stats,

		// lifecycle channels
		exitHostCh:            make(chan bool),
		stopHostInterrupt:     new(int32),
		bootstrappingComplete: new(int32),

		// incoming data
		blockRPCCh: make(chan blockAndParent),
		forkRPCCh:  make(chan []common.EncodedBlock),
		txP2PCh:    make(chan common.EncryptedTx),

		// Initialize the host DB
		// nodeDB:       NewLevelDBBackedDB(), // todo - make this config driven
		hostDB: db.NewInMemoryDB(),

		mgmtContractLib: mgmtContractLib, // library that provides a handler for Management Contract
		ethWallet:       ethWallet,       // the host's ethereum wallet
		logEventManager: events.NewLogEventManager(logger),

		logger: logger,
	}

	if config.HasClientRPCHTTP || config.HasClientRPCWebsockets {
		rpcAPIs := []rpc.API{
			{
				Namespace: APINamespaceObscuro,
				Version:   APIVersion1,
				Service:   clientapi.NewObscuroAPI(host),
				Public:    true,
			},
			{
				Namespace: APINamespaceEth,
				Version:   APIVersion1,
				Service:   clientapi.NewEthereumAPI(host),
				Public:    true,
			},
			{
				Namespace: apiNamespaceObscuroScan,
				Version:   APIVersion1,
				Service:   clientapi.NewObscuroScanAPI(host),
				Public:    true,
			},
			{
				Namespace: apiNamespaceNetwork,
				Version:   APIVersion1,
				Service:   clientapi.NewNetworkAPI(host),
				Public:    true,
			},
			{
				Namespace: apiNamespaceTest,
				Version:   APIVersion1,
				Service:   clientapi.NewTestAPI(host),
				Public:    true,
			},
			{
				Namespace: APINamespaceEth,
				Version:   APIVersion1,
				Service:   clientapi.NewFilterAPI(host, logger),
				Public:    true,
			},
		}
		host.rpcServer = clientrpc.NewServer(config, rpcAPIs, logger)
	}

	var prof *profiler.Profiler
	if config.ProfilerEnabled {
		prof = profiler.NewProfiler(profiler.DefaultHostPort, logger)
		err := prof.Start()
		if err != nil {
			logger.Crit("unable to start the profiler: %s", log.ErrKey, err)
		}
	}

	jsonConfig, _ := json.MarshalIndent(config, "", "  ")
	logger.Info("Host service created with following config:", log.CfgKey, string(jsonConfig))

	return host
}

func (h *host) Start() {
	h.validateConfig()

	tomlConfig, err := toml.Marshal(h.config)
	if err != nil {
		h.logger.Crit("could not print host config")
	}
	h.logger.Info("Host started with following config", log.CfgKey, string(tomlConfig))

	// wait for the Enclave to be available
	h.waitForEnclave()

	// todo: we should try to recover the key from a previous run of the node here? Before generating or requesting the key.
	if h.config.IsGenesis {
		err = h.broadcastSecret()
		if err != nil {
			h.logger.Crit("Could not broadcast secret", log.ErrKey, err.Error())
		}
	} else {
		err = h.requestSecret()
		if err != nil {
			h.logger.Crit("Could not request secret", log.ErrKey, err.Error())
		}
	}

	// attach the l1 monitor
	go h.monitorBlocks()

	// bootstrap the host
	latestBlock := h.bootstrapHost()

	// start the enclave speculative work from last block
	err = h.enclaveClient.Start(latestBlock)
	if err != nil {
		h.logger.Crit("Could not start the enclave.", log.ErrKey, err.Error())
	}

	if h.config.IsGenesis {
		_, err = h.initialiseProtocol(&latestBlock)
		if err != nil {
			h.logger.Crit("Could not bootstrap.", log.ErrKey, err.Error())
		}
	}
	// start the obscuro RPC endpoints
	if h.rpcServer != nil {
		h.rpcServer.Start()
		h.logger.Info("Started client server.")
	}

	// start the host's main processing loop
	h.startProcessing()
}

func (h *host) broadcastSecret() error {
	h.logger.Info("Node is genesis node. Broadcasting secret.")
	// Create the shared secret and submit it to the management contract for storage
	attestation, err := h.enclaveClient.Attestation()
	if err != nil {
		return fmt.Errorf("could not retrieve attestation from enclave. Cause: %w", err)
	}
	if attestation.Owner != h.config.ID {
		return fmt.Errorf("genesis node has ID %s, but its enclave produced an attestation using ID %s", h.config.ID.Hex(), attestation.Owner.Hex())
	}

	encodedAttestation, err := common.EncodeAttestation(attestation)
	if err != nil {
		return fmt.Errorf("could not encode attestation. Cause: %w", err)
	}

	secret, err := h.enclaveClient.GenerateSecret()
	if err != nil {
		return fmt.Errorf("could not generate secret. Cause: %w", err)
	}

	l1tx := &ethadapter.L1InitializeSecretTx{
		AggregatorID:  &h.config.ID,
		Attestation:   encodedAttestation,
		InitialSecret: secret,
		HostAddress:   h.config.P2PPublicAddress,
	}
	initialiseSecretTx := h.mgmtContractLib.CreateInitializeSecret(l1tx, h.ethWallet.GetNonceAndIncrement())
	err = h.signAndBroadcastL1Tx(initialiseSecretTx, l1TxTriesSecret)
	if err != nil {
		return fmt.Errorf("failed to initialise enclave secret. Cause: %w", err)
	}
	h.logger.Info("Node is genesis node. Secret was broadcast.")
	return nil
}

func (h *host) Config() *config.HostConfig {
	return &h.config
}

func (h *host) DB() *db.DB {
	return h.hostDB
}

func (h *host) EnclaveClient() common.Enclave {
	return h.enclaveClient
}

func (h *host) MockedNewHead(b common.EncodedBlock, p common.EncodedBlock) {
	if atomic.LoadInt32(h.stopHostInterrupt) == 1 {
		return
	}
	h.blockRPCCh <- blockAndParent{b, p}
}

func (h *host) MockedNewFork(b []common.EncodedBlock) {
	if atomic.LoadInt32(h.stopHostInterrupt) == 1 {
		return
	}
	h.forkRPCCh <- b
}

func (h *host) SubmitAndBroadcastTx(encryptedParams common.EncryptedParamsSendRawTx) (common.EncryptedResponseSendRawTx, error) {
	encryptedTx := common.EncryptedTx(encryptedParams)
	encryptedResponse, err := h.enclaveClient.SubmitTx(encryptedTx)
	if err != nil {
		h.logger.Info("Could not submit transaction", log.ErrKey, err)
		return nil, err
	}

	err = h.p2p.BroadcastTx(encryptedTx)
	if err != nil {
		return nil, fmt.Errorf("could not broadcast transaction. Cause: %w", err)
	}

	return encryptedResponse, nil
}

// ReceiveTx receives a new transaction
func (h *host) ReceiveTx(tx common.EncryptedTx) {
	if atomic.LoadInt32(h.stopHostInterrupt) == 1 {
		return
	}
	h.txP2PCh <- tx
}

func (h *host) Subscribe(id rpc.ID, encryptedLogSubscription common.EncryptedParamsLogSubscription, matchedLogsCh chan []byte) error {
	err := h.EnclaveClient().Subscribe(id, encryptedLogSubscription)
	if err != nil {
		return fmt.Errorf("could not create subscription with enclave. Cause: %w", err)
	}
	h.logEventManager.AddSubscription(id, matchedLogsCh)
	return nil
}

func (h *host) Unsubscribe(id rpc.ID) {
	err := h.EnclaveClient().Unsubscribe(id)
	if err != nil {
		h.logger.Error("could not terminate subscription", log.SubIDKey, id, log.ErrKey, err)
	}
	h.logEventManager.RemoveSubscription(id)
}

func (h *host) Stop() {
	// block all requests
	atomic.StoreInt32(h.stopHostInterrupt, 1)

	if err := h.p2p.StopListening(); err != nil {
		h.logger.Error("failed to close transaction P2P listener cleanly", log.ErrKey, err)
	}
	if err := h.enclaveClient.Stop(); err != nil {
		h.logger.Error("could not stop enclave server", log.ErrKey, err)
	}
	if err := h.enclaveClient.StopClient(); err != nil {
		h.logger.Error("failed to stop enclave RPC client", log.ErrKey, err)
	}
	if h.rpcServer != nil {
		// We cannot stop the RPC server synchronously. This is because the host itself is being stopped by an RPC
		// call, so there is a deadlock. The RPC server is waiting for all connections to close, but a single
		// connection remains open, waiting for the RPC server to close.
		go h.rpcServer.Stop()
	}

	// Leave some time for all processing to finish before exiting the main loop.
	time.Sleep(time.Second)
	h.exitHostCh <- true

	h.logger.Info("Host shut down successfully.")
}

// Waits for enclave to be available, printing a wait message every two seconds.
func (h *host) waitForEnclave() {
	counter := 0
	for _, err := h.enclaveClient.Status(); err != nil; {
		if counter >= 20 {
			h.logger.Info(fmt.Sprintf("Waiting for enclave on %s. Latest connection attempt failed", h.config.EnclaveRPCAddress), log.ErrKey, err)
			counter = 0
		}

		time.Sleep(100 * time.Millisecond)
		counter++
	}
	h.logger.Info("Connected to enclave service.")
}

// starts the host main processing loop
func (h *host) startProcessing() {
	// Only open the p2p connection when the host is fully initialised
	h.p2p.StartListening(h)

	// use the roundInterrupt as a signaling mechanism for interrupting block processing
	// stops processing the current round if a new block arrives
	i := int32(0)
	roundInterrupt := &i

	// Main Processing Loop -
	// - Process new blocks from the L1 node
	// - Process new Transactions gossiped from L2 Peers
	for {
		select {
		case b := <-h.blockRPCCh:
			roundInterrupt = triggerInterrupt(roundInterrupt)
			err := h.processBlock(b.p, false)
			if err != nil {
				h.logger.Info("Could not process parent block. ", log.ErrKey, err)
			}
			err = h.processBlock(b.b, true)
			if err != nil {
				h.logger.Warn("Could not process latest block. ", log.ErrKey, err)
			}

		case f := <-h.forkRPCCh:
			roundInterrupt = triggerInterrupt(roundInterrupt)
			for i, blk := range f {
				isLatest := i == (len(f) - 1)
				err := h.processBlock(blk, isLatest)
				if err != nil && isLatest {
					h.logger.Warn("Could not process latest fork block received via RPC.", log.ErrKey, err)
				}
			}

		case tx := <-h.txP2PCh:
			if _, err := h.enclaveClient.SubmitTx(tx); err != nil {
				h.logger.Warn("Could not submit transaction. ", log.ErrKey, err)
			}

		case <-h.exitHostCh:
			return
		}
	}
}

// activates the given interrupt (atomically) and returns a new interrupt
func triggerInterrupt(interrupt *int32) *int32 {
	// Notify the previous round to stop work
	atomic.StoreInt32(interrupt, 1)
	i := int32(0)
	return &i
}

type blockAndParent struct {
	b common.EncodedBlock
	p common.EncodedBlock
}

func (h *host) processBlock(block common.EncodedBlock, latest bool) error {
	var result *common.BlockSubmissionResponse

	// For the genesis block the parent is nil
	if block == nil {
		return nil
	}

	decoded, err := block.DecodeBlock()
	if err != nil {
		return err
	}
	h.processBlockTransactions(decoded)

	// submit each block to the enclave for ingestion plus validation
	// todo: isLatest should only be true when we're not behind
	result, err = h.enclaveClient.SubmitBlock(*decoded, latest)
	if err != nil {
		return fmt.Errorf("did not ingest block b_%d. Cause: %w", common.ShortHash(decoded.Hash()), err)
	}

	h.storeBlockProcessingResult(result)
	h.logEventManager.SendLogsToSubscribers(result)

	// We check that we only produced a rollup if we're an aggregator.
	if result.ProducedRollup.Header != nil && h.config.NodeType != common.Aggregator {
		h.logger.Crit("node produced a rollup but was not an aggregator")
	}

	for _, secretResponse := range result.ProducedSecretResponses {
		err := h.publishSharedSecretResponse(secretResponse)
		if err != nil {
			h.logger.Error("failed to publish response to secret request", log.ErrKey, err)
		}
	}

	if latest {
		if result.ProducedRollup.Header == nil {
			return nil
		}
		h.publishRollup(result.ProducedRollup)
	}
	return nil
}

// Looks at each transaction in the block, and kicks off special handling for the transaction if needed.
func (h *host) processBlockTransactions(b *types.Block) {
	for _, tx := range b.Transactions() {
		t := h.mgmtContractLib.DecodeTx(tx)
		if t == nil {
			continue
		}

		if scrtRespTx, ok := t.(*ethadapter.L1RespondSecretTx); ok {
			err := h.processSharedSecretResponse(scrtRespTx)
			if err != nil {
				h.logger.Error("Failed to process shared secret response", log.ErrKey, err)
				continue
			}
		}
	}
}

func (h *host) publishRollup(producedRollup common.ExtRollup) {
	if atomic.LoadInt32(h.stopHostInterrupt) == 1 {
		return
	}

	encodedRollup, err := common.EncodeRollup(producedRollup.ToExtRollupWithHash())
	if err != nil {
		h.logger.Crit("could not encode rollup.", log.ErrKey, err)
	}
	tx := &ethadapter.L1RollupTx{
		Rollup: encodedRollup,
	}

	rollupTx := h.mgmtContractLib.CreateRollup(tx, h.ethWallet.GetNonceAndIncrement())
	err = h.signAndBroadcastL1Tx(rollupTx, l1TxTriesRollup)
	if err != nil {
		h.logger.Error("could not broadcast rollup", log.ErrKey, err)
	}
}

func (h *host) storeBlockProcessingResult(result *common.BlockSubmissionResponse) {
	// only update the host rollup headers if the enclave has found a new rollup head
	if result.FoundNewHead {
		// adding a header will update the head if it has a higher height
		headerWithHashes := common.HeaderWithTxHashes{Header: result.RollupHead, TxHashes: result.ProducedRollup.TxHashes}
		h.hostDB.AddRollupHeader(&headerWithHashes)
	}

	// adding a header will update the head if it has a higher height
	h.hostDB.AddBlockHeader(result.BlockHeader)
}

// Called only by the first enclave to bootstrap the network
func (h *host) initialiseProtocol(block *types.Block) (common.L2RootHash, error) {
	// Create the genesis rollup and submit it to the management contract
	genesisResponse, err := h.enclaveClient.ProduceGenesis(block.Hash())
	if err != nil {
		return common.L2RootHash{}, fmt.Errorf("could not produce genesis. Cause: %w", err)
	}
	h.logger.Info(
		fmt.Sprintf("Initialising network. Genesis rollup r_%d.",
			common.ShortHash(genesisResponse.ProducedRollup.Header.Hash()),
		))
	encodedRollup, err := common.EncodeRollup(genesisResponse.ProducedRollup.ToExtRollupWithHash())
	if err != nil {
		return common.L2RootHash{}, fmt.Errorf("could not encode rollup. Cause: %w", err)
	}
	l1tx := &ethadapter.L1RollupTx{
		Rollup: encodedRollup,
	}

	rollupTx := h.mgmtContractLib.CreateRollup(l1tx, h.ethWallet.GetNonceAndIncrement())
	err = h.signAndBroadcastL1Tx(rollupTx, l1TxTriesRollup)
	if err != nil {
		return common.L2RootHash{}, fmt.Errorf("could not initialise protocol. Cause: %w", err)
	}

	return genesisResponse.ProducedRollup.Header.ParentHash, nil
}

// `tries` is the number of times to attempt broadcasting the transaction.
func (h *host) signAndBroadcastL1Tx(tx types.TxData, tries uint64) error {
	signedTx, err := h.ethWallet.SignTransaction(tx)
	if err != nil {
		return err
	}

	err = retry.Do(func() error {
		return h.ethClient.SendTransaction(signedTx)
	}, retry.NewDoublingBackoffStrategy(time.Second, tries)) // doubling retry wait (3 tries = 7sec, 7 tries = 63sec)
	if err != nil {
		return fmt.Errorf("broadcasting L1 transaction failed after %d tries. Cause: %w", tries, err)
	}
	h.logger.Trace("L1 transaction sent successfully, watching for receipt.")

	// asynchronously watch for a successful receipt
	// todo: consider how to handle the various ways that L1 transactions could fail to improve node operator QoL
	go h.watchForReceipt(signedTx.Hash())

	return nil
}

func (h *host) watchForReceipt(txHash common.TxHash) {
	var receipt *types.Receipt
	var err error
	err = retry.Do(func() error {
		receipt, err = h.ethClient.TransactionReceipt(txHash)
		return err
	}, retry.NewTimeoutStrategy(maxWaitForL1Receipt, retryIntervalForL1Receipt))
	if err != nil {
		h.logger.Error("receipt for L1 transaction never found despite 'successful' broadcast",
			"err", err,
			"signer", h.ethWallet.Address().Hex())
	}
	if err == nil && receipt.Status != types.ReceiptStatusSuccessful {
		h.logger.Error("unsuccessful receipt found for published L1 transaction",
			"status", receipt.Status,
			"signer", h.ethWallet.Address().Hex())
	}
	h.logger.Trace("Successful L1 transaction receipt found.", "blk", receipt.BlockNumber, "blkHash", receipt.BlockHash)
}

// This method implements the procedure by which a node obtains the secret
func (h *host) requestSecret() error {
	h.logger.Info("Requesting secret.")
	att, err := h.enclaveClient.Attestation()
	if err != nil {
		return fmt.Errorf("could not retrieve attestation from enclave. Cause: %w", err)
	}
	if att.Owner != h.config.ID {
		return fmt.Errorf("host has ID %s, but its enclave produced an attestation using ID %s", h.config.ID.Hex(), att.Owner.Hex())
	}
	encodedAttestation, err := common.EncodeAttestation(att)
	if err != nil {
		return fmt.Errorf("could not encode attestation. Cause: %w", err)
	}
	l1tx := &ethadapter.L1RequestSecretTx{
		Attestation: encodedAttestation,
	}
	requestSecretTx := h.mgmtContractLib.CreateRequestSecret(l1tx, h.ethWallet.GetNonceAndIncrement())
	err = h.signAndBroadcastL1Tx(requestSecretTx, l1TxTriesSecret)
	if err != nil {
		return err
	}

	err = h.awaitSecret()
	if err != nil {
		h.logger.Crit("could not receive the secret", log.ErrKey, err)
	}
	return nil
}

func (h *host) handleStoreSecretTx(t *ethadapter.L1RespondSecretTx) bool {
	if t.RequesterID.Hex() != h.config.ID.Hex() {
		// this secret is for somebody else
		return false
	}

	// someone has replied for us
	err := h.enclaveClient.InitEnclave(t.Secret)
	if err != nil {
		h.logger.Info("Failed to initialise enclave with received secret.", log.ErrKey, err.Error())
		return false
	}
	return true
}

func (h *host) publishSharedSecretResponse(scrtResponse *common.ProducedSecretResponse) error {
	// todo: implement proper protocol so only one host responds to this secret requests initially
	// 	for now we just have the genesis host respond until protocol implemented
	if !h.config.IsGenesis {
		h.logger.Trace("Not genesis node, not publishing response to secret request.",
			"requester", scrtResponse.RequesterID)
		return nil
	}

	l1tx := &ethadapter.L1RespondSecretTx{
		Secret:      scrtResponse.Secret,
		RequesterID: scrtResponse.RequesterID,
		AttesterID:  h.config.ID,
		HostAddress: scrtResponse.HostAddress,
	}
	// TODO review: l1tx.Sign(a.attestationPubKey) doesn't matter as the waitSecret will process a tx that was reverted
	respondSecretTx := h.mgmtContractLib.CreateRespondSecret(l1tx, h.ethWallet.GetNonceAndIncrement(), false)
	h.logger.Trace("Broadcasting secret response L1 tx.", "requester", scrtResponse.RequesterID)
	err := h.signAndBroadcastL1Tx(respondSecretTx, l1TxTriesSecret)
	if err != nil {
		return fmt.Errorf("could not broadcast secret response. Cause %w", err)
	}
	return nil
}

// Whenever we receive a new shared secret response transaction, we update our list of P2P peers, as another aggregator
// may have joined the network.
func (h *host) processSharedSecretResponse(_ *ethadapter.L1RespondSecretTx) error {
	// We make a call to the L1 node to retrieve the new list of aggregators. An alternative would be to check that the
	// transaction succeeded, and if so, extract the additional host address from the transaction arguments. But we
	// believe this would be more brittle than just asking the L1 contract for its view of the current aggregators.
	msg, err := h.mgmtContractLib.GetHostAddresses()
	if err != nil {
		return err
	}
	response, err := h.ethClient.CallContract(msg)
	if err != nil {
		return err
	}
	decodedResponse, err := h.mgmtContractLib.DecodeCallResponse(response)
	if err != nil {
		return err
	}
	hostAddresses := decodedResponse[0]

	// We filter down the list of retrieved addresses.
	var filteredHostAddresses []string //nolint:prealloc
	for _, hostAddress := range hostAddresses {
		// We exclude our own address.
		if hostAddress == h.config.P2PPublicAddress {
			continue
		}

		// We exclude any duplicate host addresses.
		isDup := false
		for _, existingHostAddress := range filteredHostAddresses {
			if hostAddress == existingHostAddress {
				isDup = true
				break
			}
		}
		if isDup {
			continue
		}

		filteredHostAddresses = append(filteredHostAddresses, hostAddress)
	}

	h.p2p.UpdatePeerList(filteredHostAddresses)
	return nil
}

// monitors the L1 client for new blocks and injects them into the aggregator
func (h *host) monitorBlocks() {
	var lastKnownBlkHash gethcommon.Hash
	listener, subs := h.ethClient.BlockListener()
	h.logger.Info("Start monitoring Ethereum blocks..")

	// only process blocks if the host is running
	for atomic.LoadInt32(h.stopHostInterrupt) == 0 {
		select {
		case err := <-subs.Err():
			h.logger.Error("L1 block monitoring error", log.ErrKey, err)
			h.logger.Info("Restarting L1 block Monitoring...")
			// it's fine to immediately restart the listener, any incoming blocks will be on hold in the queue
			listener, subs = h.ethClient.BlockListener()

			err = h.catchupMissedBlocks(lastKnownBlkHash)
			if err != nil {
				h.logger.Crit("could not catch up missed blocks.", log.ErrKey, err)
			}

		case blkHeader := <-listener:
			// don't process blocks if the host is stopping
			if atomic.LoadInt32(h.stopHostInterrupt) == 1 {
				break
			}

			// ignore blocks if bootstrapping is happening
			if atomic.LoadInt32(h.bootstrappingComplete) == 0 {
				h.logger.Trace(fmt.Sprintf("Host in bootstrap - ignoring block %s", blkHeader.Hash()))
				continue
			}

			block, err := h.ethClient.BlockByHash(blkHeader.Hash())
			if err != nil {
				h.logger.Crit(fmt.Sprintf("could not fetch block for hash %s.", blkHeader.Hash().String()), log.ErrKey, err)
			}
			blockParent, err := h.ethClient.BlockByHash(block.ParentHash())
			if err != nil {
				h.logger.Crit(fmt.Sprintf("could not fetch block's parent with hash %s.", block.ParentHash().String()), log.ErrKey, err)
			}

			h.logger.Info(fmt.Sprintf(
				"Received a new block b_%d(%d)",
				common.ShortHash(blkHeader.Hash()),
				blkHeader.Number.Uint64(),
			))

			// issue the block to the ingestion channel
			err = h.encodeAndIngest(block, blockParent)
			if err != nil {
				h.logger.Crit("Internal error", log.ErrKey, err)
			}
			lastKnownBlkHash = block.Hash()
		}
	}

	h.logger.Info("Stopped monitoring for l1 blocks")
	// make sure it cleanly unsubscribes
	// todo this should be defered when the errors are upstreamed instead of panic'd
	subs.Unsubscribe()
}

func (h *host) catchupMissedBlocks(lastKnownBlkHash gethcommon.Hash) error {
	var lastBlkNumber *big.Int
	var reingestBlocks []*types.Block

	// get the blockchain tip block
	blk, err := h.ethClient.BlockByNumber(lastBlkNumber)
	if err != nil {
		return fmt.Errorf("catching up on missed blocks, unable to fetch tip block - reason: %w", err)
	}

	if blk.Hash().Hex() == lastKnownBlkHash.Hex() {
		// if no new blocks have been issued then nothing to catchup
		return nil
	}
	reingestBlocks = append(reingestBlocks, blk)

	// get all blocks from the blockchain tip to the last block ingested by the host
	for blk.Hash().Hex() != lastKnownBlkHash.Hex() {
		blockParent, err := h.ethClient.BlockByHash(blk.ParentHash())
		if err != nil {
			return fmt.Errorf("catching up on missed blocks, could not fetch block's parent with hash %s. Cause: %w", blk.ParentHash(), err)
		}

		reingestBlocks = append(reingestBlocks, blockParent)
		blk = blockParent
	}

	// make sure to have the last ingested block available for ingestion (because we always ingest ( blk, blk_parent)
	lastKnownBlk, err := h.ethClient.BlockByHash(lastKnownBlkHash)
	if err != nil {
		return fmt.Errorf("catching up on missed blocks, unable to feth last known block - reason: %w", err)
	}
	reingestBlocks = append(reingestBlocks, lastKnownBlk)

	// issue the block to the ingestion channel in reverse, with the parent attached too
	for i := len(reingestBlocks) - 2; i >= 0; i-- {
		h.logger.Debug(fmt.Sprintf("Ingesting %s and %s blocks of %v", reingestBlocks[i].Hash(), reingestBlocks[i+1].Hash(), reingestBlocks))
		err = h.encodeAndIngest(reingestBlocks[i], reingestBlocks[i+1])
		if err != nil {
			h.logger.Crit("Internal error", log.ErrKey, err)
		}
	}

	return nil
}

func (h *host) encodeAndIngest(block *types.Block, blockParent *types.Block) error {
	encodedBlock, err := common.EncodeBlock(block)
	if err != nil {
		return fmt.Errorf("could not encode block with hash %s. Cause: %w", block.Hash().String(), err)
	}

	encodedBlockParent, err := common.EncodeBlock(blockParent)
	if err != nil {
		return fmt.Errorf("could not encode block's parent with hash %s. Cause: %w", block.ParentHash().String(), err)
	}

	h.blockRPCCh <- blockAndParent{encodedBlock, encodedBlockParent}
	return nil
}

func (h *host) bootstrapHost() types.Block {
	var err error
	var nextBlk *types.Block

	// build up from the genesis block
	// todo update to bootstrap from the last block in storage
	// todo the genesis block should be the block where the contract was deployed
	currentBlock, err := h.ethClient.BlockByNumber(big.NewInt(0))
	if err != nil {
		h.logger.Crit("Internal error", log.ErrKey, err)
	}

	h.logger.Info(fmt.Sprintf("Started host bootstrap with block %d", currentBlock.NumberU64()))

	startTime, logTime := time.Now(), time.Now()
	for {
		cb := *currentBlock
		h.processBlockTransactions(&cb)
		result, err := h.enclaveClient.SubmitBlock(cb, false)
		if err != nil {
			var bsErr *common.BlockRejectError
			isBSE := errors.As(err, bsErr)
			if !isBSE {
				// unexpected error
				h.logger.Crit("Internal error", log.ErrKey, err)
			}
			// todo: we need to use the latest hash info from the BlockRejectError to realign the block streaming for the enclave
			h.logger.Info(fmt.Sprintf("Failed to ingest block b_%d. Cause: %s",
				common.ShortHash(result.BlockHeader.Hash()), bsErr))
		} else {
			// submission was successful
			h.storeBlockProcessingResult(result)
		}

		nextBlk, err = h.ethClient.BlockByNumber(big.NewInt(cb.Number().Int64() + 1))
		if err != nil {
			if errors.Is(err, ethereum.NotFound) {
				break
			}
			h.logger.Crit("Internal error", log.ErrKey, err)
		}
		currentBlock = nextBlk

		if time.Since(logTime) > 30*time.Second {
			h.logger.Info(fmt.Sprintf("Bootstrapping host at block... %d", cb.NumberU64()))
			logTime = time.Now()
		}
	}
	atomic.StoreInt32(h.bootstrappingComplete, 1)
	h.logger.Info(fmt.Sprintf("Finished bootstrap process with block %d after %s",
		currentBlock.NumberU64(),
		time.Since(startTime),
	))
	return *currentBlock
}

func (h *host) awaitSecret() error {
	// start listening for l1 blocks that contain the response to the request
	listener, subs := h.ethClient.BlockListener()

	for {
		select {
		case err := <-subs.Err():
			h.logger.Error("Restarting L1 block monitoring while awaiting for secret. Errored with: %s", log.ErrKey, err)
			// todo this is a very simple way of reconnecting the host, it might need catching up logic
			listener, subs = h.ethClient.BlockListener()

		// todo: find a way to get rid of this case and only listen for blocks on the expected channels
		case header := <-listener:
			block, err := h.ethClient.BlockByHash(header.Hash())
			if err != nil {
				return fmt.Errorf("failed to retrieve block. Cause: %w", err)
			}
			if h.checkBlockForSecretResponse(block) {
				// todo this should be deferred when the errors are upstreamed instead of panic'd
				subs.Unsubscribe()
				return nil
			}

		case bAndParent := <-h.blockRPCCh:
			block, err := bAndParent.b.DecodeBlock()
			if err != nil {
				return fmt.Errorf("failed to decode block received via RPC. Cause: %w", err)
			}
			if h.checkBlockForSecretResponse(block) {
				subs.Unsubscribe()
				return nil
			}

		case <-time.After(time.Second * 10):
			// This will provide useful feedback if things are stuck (and in tests if any goroutines got stranded on this select)
			h.logger.Info("Still waiting for secret from the L1...")

		case <-h.exitHostCh:
			subs.Unsubscribe()
			return nil
		}
	}
}

func (h *host) checkBlockForSecretResponse(block *types.Block) bool {
	for _, tx := range block.Transactions() {
		t := h.mgmtContractLib.DecodeTx(tx)
		if t == nil {
			continue
		}
		if scrtTx, ok := t.(*ethadapter.L1RespondSecretTx); ok {
			ok := h.handleStoreSecretTx(scrtTx)
			if ok {
				h.logger.Info("Stored enclave secret.")
				return true
			}
		}
	}
	// response not found
	return false
}

// Checks the host config is valid.
func (h *host) validateConfig() {
	if h.config.IsGenesis && h.config.NodeType != common.Aggregator {
		h.logger.Crit("genesis node must be an aggregator")
	}
	if !h.config.IsGenesis && h.config.NodeType == common.Aggregator {
		h.logger.Crit("only the genesis node can be an aggregator")
	}

	if h.config.P2PPublicAddress == "" {
		h.logger.Crit("the host must specify a public P2P address")
	}
}
