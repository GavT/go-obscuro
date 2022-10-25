package ethadapter

import (
	"context"
	"errors"
	"fmt"
	gethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	gethlog "github.com/ethereum/go-ethereum/log"
	"github.com/obscuronet/go-obscuro/go/common"
	"github.com/obscuronet/go-obscuro/go/common/log"
	"math/big"
	"sync"
	"time"
)

const (
	defaultLiveBuffer = 50
)

var (
	errBlockNotFound = errors.New("block not found")
	zero             = big.NewInt(0)
	one              = big.NewInt(1)
)

type EthBlockProvider struct {
	ethClient EthClient
	ctx       context.Context

	liveBlocks *LiveBlocksBuffer // buffer of streamed live blocks that haven't been sent yet

	streamCh chan *types.Block
	errCh    chan error
	stopCh   chan bool

	sentChainHead *common.BlockChainHead // the height and hashes of blocks sent recently, set to nil when streamFrom resets
	streamFrom    *big.Int               // height most-recently requested to stream from

	logger gethlog.Logger
}

func (e *EthBlockProvider) Start() {
	e.liveBlocks = &LiveBlocksBuffer{
		ctx:       e.ctx,
		ethClient: e.ethClient,
		queueCap:  defaultLiveBuffer,
	}
	go e.liveBlocks.Start() // process to handle incoming stream of L1 blocks
}

func (e *EthBlockProvider) Stream() <-chan *types.Block {
	return e.streamCh
}

func (e *EthBlockProvider) Err() <-chan error {
	return e.errCh
}

// StartStreamingFrom will (re)start streaming blocks from the specified height (or earlier if recentHashes don't match the canonical chain)
// if streaming was already in progress then any blocks in-flight will be discarded
func (e *EthBlockProvider) StartStreamingFrom(blockHead common.BlockChainHead) {
	updateStreamFrom := blockHead.Height + 1

	if e.sentChainHead != nil {
		if latestCommonAncestor, found := blockHead.LatestCommonAncestor(e.sentChainHead); found {
			updateStreamFrom = latestCommonAncestor + 1
		}
	}
	firstReq := e.streamFrom == nil
	e.streamFrom = big.NewInt(int64(updateStreamFrom))

	if firstReq {
		// the first time this is called we kick off the process to stream outbound blocks
		go e.streamBlocks()
	}
}

func (e *EthBlockProvider) Stop() {
	e.stopCh <- true
}

func (e *EthBlockProvider) IsLive(h gethcommon.Hash) bool {
	return false //return h == e.sentChainHead.
}

// streamBlocks should be run in a separate go routine. It will stream catch-up blocks from requested height until it
// reaches the live block buffer and then feed from there.
// It blocks when:
// - publishing a block, it blocks on the outbound channel until the block is consumed
// - awaiting a live block, when consumer is completely up-to-date it waits for a live block to arrive
func (e *EthBlockProvider) streamBlocks() {
	for !e.stopped() {
		block, err := e.nextBlockToStream()
		if err != nil {
			e.logger.Error("unexpected error while preparing block to stream, will retry in 1 sec - %s", err)
			time.Sleep(time.Second)
			continue
		}
		e.streamFrom = block.Number().Add(block.Number(), one)
		println("sending!")
		e.streamCh <- block
	}
}

func (e *EthBlockProvider) nextBlockToStream() (*types.Block, error) {
	var block *types.Block
	var err error
	heightToStream := e.streamFrom // we take a copy of this variable because we want it to remain consistent for this logical block (but we don't mind if we publish an out-of-date one)

	// we want to know where the live blocks are at so we know if we need to do lookups for historic blocks or just play the live ones
	liveBlk, found := e.liveBlocks.Peek()

	if !found {
		// if no live block found we've probably drained the live buffer, and we're waiting for the next one
		liveBlk, found = e.liveBlocks.AwaitBlockPeek(e.ctx)
		if !found {
			return nil, fmt.Errorf("await live block failed")
		}
	}

	for heightToStream.Cmp(liveBlk.Number()) > 0 {
		e.logger.Trace("Discarding blocks from live buffer to skip to target height.",
			"targetHeight", heightToStream,
			"discardedHeight", liveBlk.Number())
		// the height we want to stream from is ahead of the live block buffer, discard buffered blocks until we reach target
		// note: this shouldn't happen in expected usage but just for completeness
		_, err = e.liveBlocks.Pop()
		if err != nil {
			return nil, fmt.Errorf("live block buffer is behind target of %d, failed to pop blocks to catch up - %w", heightToStream, err)
		}
		liveBlk, found = e.liveBlocks.Peek()

	}

	if heightToStream.Cmp(liveBlk.Number()) < 0 {
		// live blocks are ahead of height we should be streaming, need to lookup by height
		block, err = e.ethClient.BlockByNumber(heightToStream)
		if err != nil {
			return nil, fmt.Errorf("no block found for height: %d - %w", heightToStream, err)
		}
	} else {
		// consumer is expecting the next live block
		block, err = e.liveBlocks.Pop()
		if err != nil {
			return nil, fmt.Errorf("unexpected error, live block should have been available - %w", err)
		}
	}

	return block, nil
}

// checkStopped checks the stopCh for a signal to stop. This seems dumb, what's the correct way to do this?
func (e *EthBlockProvider) stopped() bool {
	for {
		select {
		case <-e.stopCh:
			return true
		default:
			return false
		}
	}
}

// LiveBlocksBuffer manages a process that queues up the latest X blocks (where X=queueCap) in a FIFO queue, streamed from
// an L1 client. If it has X and receives another one it will drop the oldest.
type LiveBlocksBuffer struct {
	ctx       context.Context
	ethClient EthClient

	// we use a ring buffer to store the latest X blocks
	queue       []*types.Block
	queueCap    int
	latestIndex int
	oldestIndex int

	awaitChan chan struct{} // closes when a block is added to the buffer

	mu sync.Mutex
}

func (l *LiveBlocksBuffer) Start() {
	l.queue = make([]*types.Block, l.queueCap)
	l.oldestIndex = 0
	l.latestIndex = -1
	l.StartProcessing()
}

func (l *LiveBlocksBuffer) StartProcessing() {
	blkHeadChan, blkSubs := l.ethClient.BlockListener()
	for {
		select {
		case <-l.ctx.Done():
			return

		case blkHead := <-blkHeadChan:
			block, err := l.ethClient.BlockByHash(blkHead.Hash())
			if err != nil {
				log.Panic("could not fetch block for hash %s. Cause: %s", blkHead.Hash().String(), err)
			}
			l.Push(block)

		case err := <-blkSubs.Err():
			log.Error("L1 block monitoring error: %s", err)
			log.Info("Restarting L1 block Monitoring...")
			// this disconnect could result in a gap in the LiveBlocksBuffer queue, the block provider is responsible for
			// sending a coherent ordering of blocks and will look up blocks if parent not sent
			blkHeadChan, blkSubs = l.ethClient.BlockListener()
		}
	}
}

func (l *LiveBlocksBuffer) Length() int {
	return l.latestIndex - l.oldestIndex + 1
}

// Push adds a block to the FIFO buffer queue
func (l *LiveBlocksBuffer) Push(blk *types.Block) {
	l.mu.Lock()
	defer l.mu.Unlock()
	emptyBeforePush := l.Length() == 0

	l.latestIndex = (l.latestIndex + 1) % l.queueCap
	l.queue[l.latestIndex] = blk

	// if oldestIndex == latestIndex then either we were full or empty before push, if full then oldest index has to move on
	if l.latestIndex == l.oldestIndex && !emptyBeforePush {
		l.oldestIndex = (l.oldestIndex + 1) % l.queueCap
	}

	// notify if client was waiting
	if l.awaitChan != nil {
		close(l.awaitChan)
		l.awaitChan = nil
	}
}

// Pop retrieves the oldest block from the buffer and removes it from the buffer
func (l *LiveBlocksBuffer) Pop() (*types.Block, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.Length() == 0 {
		return nil, errBlockNotFound
	}
	oldestBlock := l.queue[l.oldestIndex]
	l.oldestIndex = (l.oldestIndex + 1) % l.queueCap
	println("§§ popped!")
	return oldestBlock, nil
}

// Peek retrieves the oldest block from the buffer but leaves it on the buffer, and a found bool (false if buffer empty)
func (l *LiveBlocksBuffer) Peek() (*types.Block, bool) {
	if l.Length() == 0 {
		return nil, false
	}
	return l.queue[l.oldestIndex], true
}

func (l *LiveBlocksBuffer) AwaitBlockPeek(ctx context.Context) (*types.Block, bool) {
	awaitChan := l.registerAwait()
	for {
		select {
		case <-awaitChan:
			return l.Peek()
		case <-ctx.Done():
			return nil, false
		}
	}
}

func (l *LiveBlocksBuffer) registerAwait() <-chan struct{} {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.awaitChan = make(chan struct{})
	if _, found := l.Peek(); found {
		close(l.awaitChan)
	}

	return l.awaitChan
}
