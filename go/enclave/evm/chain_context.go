package evm

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/obscuronet/go-obscuro/go/enclave/db"
)

// ObscuroChainContext - basic implementation of the ChainContext needed for the EVM integration
type ObscuroChainContext struct {
	storage db.Storage
}

func (*ObscuroChainContext) Engine() consensus.Engine {
	return &ObscuroNoOpConsensusEngine{}
}

func (occ *ObscuroChainContext) GetHeader(hash common.Hash, height uint64) *types.Header {
	rol, f := occ.storage.FetchRollup(hash)

	if !f {
		return nil
	}
	return convertToEthHeader(rol.Header, secret(occ.storage))
}
