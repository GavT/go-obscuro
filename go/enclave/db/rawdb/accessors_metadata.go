package rawdb

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethdb"
	gethlog "github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/obscuronet/go-obscuro/go/common/log"
	"github.com/obscuronet/go-obscuro/go/enclave/crypto"
)

func ReadSharedSecret(db ethdb.KeyValueReader) *crypto.SharedEnclaveSecret {
	var ss crypto.SharedEnclaveSecret

	enc, _ := db.Get(sharedSecret)
	if len(enc) == 0 {
		return nil
	}
	if err := rlp.DecodeBytes(enc, &ss); err != nil {
		return nil
	}

	return &ss
}

func WriteSharedSecret(db ethdb.KeyValueWriter, ss crypto.SharedEnclaveSecret, logger gethlog.Logger) {
	enc, err := rlp.EncodeToBytes(ss)
	if err != nil {
		logger.Crit("could not encode shared secret. ", log.ErrKey, err)
	}
	if err = db.Put(sharedSecret, enc); err != nil {
		logger.Crit("could not put shared secret in DB. ", log.ErrKey, err)
	}
}

func ReadGenesisHash(db ethdb.KeyValueReader) *common.Hash {
	var hash common.Hash

	enc, _ := db.Get(genesisRollupHash)
	if len(enc) == 0 {
		return nil
	}
	if err := rlp.DecodeBytes(enc, &hash); err != nil {
		return nil
	}

	return &hash
}

func WriteGenesisHash(db ethdb.KeyValueWriter, hash common.Hash, logger gethlog.Logger) {
	enc, err := rlp.EncodeToBytes(hash)
	if err != nil {
		logger.Crit("could not encode genesis hash. ", log.ErrKey, err)
	}
	if err = db.Put(genesisRollupHash, enc); err != nil {
		logger.Crit("could not put genesis hash in DB. ", log.ErrKey, err)
	}
}
