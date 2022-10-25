package ethadapter

import (
	"context"
	"fmt"
	"github.com/ethereum/go-ethereum"
	gethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/obscuronet/go-obscuro/go/common"
	"github.com/stretchr/testify/mock"
	"math/big"
	"testing"
	"time"
)

var (
	hashes = []gethcommon.Hash{
		gethcommon.HexToHash("0"),
		gethcommon.HexToHash("1"),
		gethcommon.HexToHash("2"),
		gethcommon.HexToHash("3"),
		gethcommon.HexToHash("4"),
		gethcommon.HexToHash("5"),
		gethcommon.HexToHash("6"),
		gethcommon.HexToHash("7"),
		gethcommon.HexToHash("8"),
	}
)

func TestBlockProviderHappyPathLiveStream(t *testing.T) {
	mockEthClient := mockEthClient()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	blockProvider := EthBlockProvider{
		ethClient: mockEthClient,
		ctx:       ctx,
		streamCh:  make(chan *types.Block),
	}

	blockProvider.Start()
	blkStream := blockProvider.Stream()
	time.Sleep(20 * time.Millisecond)
	bch := common.BlockChainHead{
		Height: 0,
	}
	blockProvider.StartStreamingFrom(bch)
	blkCount := 0

	for blkCount < 3 {
		select {
		case blk := <-blkStream:
			if blk != nil {
				blkCount++
			}

		case err := <-blockProvider.Err():
			t.Errorf("unexpected error: %s", err)

		case <-time.After(3 * time.Second): // shouldn't have >1sec delay between blocks in this test
			t.Errorf("expected 3 blocks from stream but got %d", blkCount)

		}
	}
}

func TestBlockProviderHappyPathHistoricThenStream(t *testing.T) {
	mockEthClient := mockEthClient()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	blockProvider := EthBlockProvider{
		ethClient: mockEthClient,
		ctx:       ctx,
		streamCh:  make(chan *types.Block),
	}

	blockProvider.Start()
	blkStream := blockProvider.Stream()
	time.Sleep(20 * time.Millisecond)
	bch := common.BlockChainHead{
		Height: 2,
	}
	blockProvider.StartStreamingFrom(bch)
	blkCount := 0

	for blkCount < 3 {
		select {
		case blk := <-blkStream:
			if blk != nil {
				blkCount++
			}

		case err := <-blockProvider.Err():
			t.Errorf("unexpected error: %s", err)

		case <-time.After(3 * time.Second): // shouldn't have >1sec delay between blocks in this test
			t.Errorf("expected 3 blocks from stream but got %d", blkCount)

		}
	}
}

func mockEthClient() EthClient {
	return ethClientMock{
		ctx:  context.TODO(),
		blks: map[gethcommon.Hash]*types.Block{},
	}
}

type ethClientMock struct {
	mock.Mock
	ctx  context.Context
	blks map[gethcommon.Hash]*types.Block
}

func (r ethClientMock) BlockListener() (chan *types.Header, ethereum.Subscription) {
	headChan := make(chan *types.Header)
	sub := &ethSubscriptionMock{}
	go func() {
		blkNum := 0
		for {
			select {
			case <-r.ctx.Done():
				return
			case <-time.After(800 * time.Millisecond):
				blkNum++
				blkHead := &types.Header{
					ParentHash: hashes[blkNum-1],
					Root:       hashes[blkNum],
					TxHash:     hashes[blkNum],
					Number:     big.NewInt(int64(blkNum)),
				}
				block := types.NewBlock(blkHead, nil, nil, nil, nil)
				r.blks[blkHead.Hash()] = block
				headChan <- blkHead
			}
		}
	}()
	return headChan, sub
}

func (r ethClientMock) BlockByHash(id gethcommon.Hash) (*types.Block, error) {
	block, f := r.blks[id]
	if !f {
		return nil, fmt.Errorf("block not found")
	}
	return block, nil
}

func (r ethClientMock) BlockByNumber(n *big.Int) (*types.Block, error) {
	//TODO implement me
	panic("implement me")
}

func (r ethClientMock) SendTransaction(signedTx *types.Transaction) error {
	//TODO implement me
	panic("implement me")
}

func (r ethClientMock) TransactionReceipt(hash gethcommon.Hash) (*types.Receipt, error) {
	//TODO implement me
	panic("implement me")
}

func (r ethClientMock) Nonce(address gethcommon.Address) (uint64, error) {
	//TODO implement me
	panic("implement me")
}

func (r ethClientMock) BalanceAt(account gethcommon.Address, blockNumber *big.Int) (*big.Int, error) {
	//TODO implement me
	panic("implement me")
}

func (r ethClientMock) Info() Info {
	//TODO implement me
	panic("implement me")
}

func (r ethClientMock) FetchHeadBlock() *types.Block {
	//TODO implement me
	panic("implement me")
}

func (r ethClientMock) BlocksBetween(block *types.Block, head *types.Block) []*types.Block {
	//TODO implement me
	panic("implement me")
}

func (r ethClientMock) IsBlockAncestor(block *types.Block, proof common.L1RootHash) bool {
	//TODO implement me
	panic("implement me")
}

func (r ethClientMock) CallContract(msg ethereum.CallMsg) ([]byte, error) {
	//TODO implement me
	panic("implement me")
}

func (r ethClientMock) Stop() {
	//TODO implement me
	panic("implement me")
}

func (r ethClientMock) EthClient() *ethclient.Client {
	//TODO implement me
	panic("implement me")
}

type ethSubscriptionMock struct {
	mock.Mock
}

func (e ethSubscriptionMock) Unsubscribe() {
	//TODO implement me
	panic("implement me")
}

func (e ethSubscriptionMock) Err() <-chan error {
	return make(<-chan error)
}
