package common

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/trie"
)

var (
	GenesisHash = common.HexToHash("0000000000000000000000000000000000000000000000000000000000000000")

	// GenesisBlock - this is a hack that has to be removed ASAP.
	GenesisBlock = NewBlock(nil, common.HexToAddress("0x0"), []*types.Transaction{})

	errNoCommonAncestorFound = errors.New("no common ancestor found")
)

// NewBlock - todo - remove this ASAP
func NewBlock(parent *types.Block, nodeID common.Address, txs []*types.Transaction) *types.Block {
	parentHash := GenesisHash
	height := L1GenesisHeight
	if parent != nil {
		parentHash = parent.Hash()
		height = parent.NumberU64() + 1
	}

	header := types.Header{
		ParentHash:  parentHash,
		UncleHash:   common.Hash{},
		Coinbase:    nodeID,
		Root:        common.Hash{},
		TxHash:      common.Hash{},
		ReceiptHash: common.Hash{},
		Bloom:       types.Bloom{},
		Difficulty:  big.NewInt(0),
		Number:      big.NewInt(int64(height)),
		GasLimit:    0,
		GasUsed:     0,
		Time:        0,
		Extra:       nil,
		MixDigest:   common.Hash{},
		Nonce:       types.BlockNonce{},
		BaseFee:     nil,
	}

	return types.NewBlock(&header, txs, nil, nil, &trie.StackTrie{})
}

// BlockChainHead records current height and recent hashes for comparison between different heads
// (useful, for example, for the host to know what to provide when the enclave rejects a block and needs to catch up)
type BlockChainHead struct {
	Height       uint64
	latestHashes []common.Hash // slice of recent hashes, hash in 0 index is the hash at `height` (latest hash)
}

// LatestCommonAncestor gives the height of the latest (newest) hash that the two chains have in common,
// also returns bool for `found`, false if no common ancestor
//
// E.g. Examining these two chains, x is behind h by 1 block, and they disagree on the hash for height 131.
// h := {height: 132, latestHashes: ["0a1b2c", "7d8e9f", "4e5d6c" ...]}
// x := {height: 131, latestHashes: ["aabbcc", "4e5d6c", ...]}
// They have the height 130 hash in common so h.LatestCommonAncestor(x) == 130
func (h *BlockChainHead) LatestCommonAncestor(x *BlockChainHead) (uint64, bool) {
	if h.Height < x.Height {
		return x.LatestCommonAncestor(h)
	}
	// Now we can assume WLOG that `h` is not behind `x`
	chainDiff := int(h.Height - x.Height) // difference in blockheight between tip of h and x
	// Iterate backwards down the chain hashes (from newest to oldest hash) starting from where x is up to
	for offsetFromHeadOfH := chainDiff; offsetFromHeadOfH < len(h.latestHashes); offsetFromHeadOfH++ {
		if h.latestHashes[offsetFromHeadOfH] == x.latestHashes[offsetFromHeadOfH-chainDiff] {
			return h.Height - uint64(offsetFromHeadOfH), true
		}
	}
	return 0, false
}

func (h *BlockChainHead) UpdateLatest(b *types.Header) {

}
