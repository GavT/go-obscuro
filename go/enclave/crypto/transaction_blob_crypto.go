package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"

	gethlog "github.com/ethereum/go-ethereum/log"

	"github.com/obscuronet/go-obscuro/go/common/log"

	gethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/obscuronet/go-obscuro/go/common"
)

const (
	// RollupEncryptionKeyHex is the AES key used to encrypt and decrypt the transaction blob in rollups.
	// TODO - Replace this fixed key with derived, rotating keys.
	RollupEncryptionKeyHex = "bddbc0d46a0666ce57a466168d99c1830b0c65e052d77188f2cbfc3f6486588c"
	// NonceLength is the nonce's length in bytes for encrypting and decrypting transactions.
	NonceLength = 12
)

// TransactionBlobCrypto handles the encryption and decryption of the transaction blobs stored inside a rollup.
type TransactionBlobCrypto interface {
	Encrypt(transactions []*common.L2Tx) common.EncryptedTransactions
	Decrypt(encryptedTxs common.EncryptedTransactions) []*common.L2Tx
}

type TransactionBlobCryptoImpl struct {
	transactionCipher cipher.AEAD
	logger            gethlog.Logger
}

func NewTransactionBlobCryptoImpl(logger gethlog.Logger) TransactionBlobCrypto {
	key := gethcommon.Hex2Bytes(RollupEncryptionKeyHex)
	block, err := aes.NewCipher(key)
	if err != nil {
		logger.Crit("could not initialise AES cipher for enclave rollup key.", log.ErrKey, err)
	}
	transactionCipher, err := cipher.NewGCM(block)
	if err != nil {
		logger.Crit("could not initialise wrapper for AES cipher for enclave rollup key. ", log.ErrKey, err)
	}
	return TransactionBlobCryptoImpl{
		transactionCipher: transactionCipher,
		logger:            logger,
	}
}

// TODO - Modify this logic so that transactions with different reveal periods are in different blobs, as per the whitepaper.
func (t TransactionBlobCryptoImpl) Encrypt(transactions []*common.L2Tx) common.EncryptedTransactions {
	encodedTxs, err := rlp.EncodeToBytes(transactions)
	if err != nil {
		t.logger.Crit("could not encrypt L2 transaction.", log.ErrKey, err)
	}

	nonce := make([]byte, NonceLength)
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		t.logger.Crit("could not generate nonce to encrypt transactions.", log.ErrKey, err)
	}

	// TODO - Ensure this nonce is not used too many times (2^32?) with the same key, to avoid risk of repeat.
	ciphertext := t.transactionCipher.Seal(nil, nonce, encodedTxs, nil)
	// We prepend the nonce to the ciphertext, so that it can be retrieved when decrypting.
	return append(nonce, ciphertext...) //nolint:makezero
}

func (t TransactionBlobCryptoImpl) Decrypt(encryptedTxs common.EncryptedTransactions) []*common.L2Tx {
	// The nonce is prepended to the ciphertext.
	nonce := encryptedTxs[0:NonceLength]
	ciphertext := encryptedTxs[NonceLength:]

	encodedTxs, err := t.transactionCipher.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		t.logger.Crit("could not decrypt encrypted L2 transactions.", log.ErrKey, err)
	}

	var txs []*common.L2Tx
	if err := rlp.DecodeBytes(encodedTxs, &txs); err != nil {
		t.logger.Crit("could not decode encoded L2 transactions.", log.ErrKey, err)
	}

	return txs
}
