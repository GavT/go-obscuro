package rpc

import (
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/rpc"

	gethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/obscuronet/go-obscuro/go/common"
	"github.com/obscuronet/go-obscuro/go/common/rpc/generated"
)

// Functions to convert classes that need to be sent between the host and the enclave to and from their equivalent
// Protobuf message classes.

func ToAttestationReportMsg(report *common.AttestationReport) generated.AttestationReportMsg {
	return generated.AttestationReportMsg{Report: report.Report, PubKey: report.PubKey, Owner: report.Owner.Bytes(), HostAddress: report.HostAddress}
}

func FromAttestationReportMsg(msg *generated.AttestationReportMsg) *common.AttestationReport {
	return &common.AttestationReport{
		Report:      msg.Report,
		PubKey:      msg.PubKey,
		Owner:       gethcommon.BytesToAddress(msg.Owner),
		HostAddress: msg.HostAddress,
	}
}

func ToBlockSubmissionResponseMsg(response *common.BlockSubmissionResponse) (generated.BlockSubmissionResponseMsg, error) {
	producedRollupMsg := ToExtRollupMsg(&response.ProducedRollup)

	subscribedLogBytes, err := json.Marshal(response.SubscribedLogs)
	if err != nil {
		return generated.BlockSubmissionResponseMsg{}, fmt.Errorf("could not marshal subscribed logs to JSON. Cause: %w", err)
	}

	return generated.BlockSubmissionResponseMsg{
		BlockHeader:             ToBlockHeaderMsg(response.BlockHeader),
		ProducedRollup:          &producedRollupMsg,
		IngestedNewRollup:       response.FoundNewHead,
		RollupHead:              ToRollupHeaderMsg(response.RollupHead),
		SubscribedLogs:          subscribedLogBytes,
		ProducedSecretResponses: ToSecretRespMsg(response.ProducedSecretResponses),
	}, nil
}

func ToBlockSubmissionRejectionMsg(rejectError *common.BlockRejectError) (generated.BlockSubmissionResponseMsg, error) {
	errMsg := &generated.BlockSubmissionErrorMsg{
		Cause:  rejectError.Wrapped.Error(),
		L1Head: rejectError.L1Head.Bytes(),
	}
	return generated.BlockSubmissionResponseMsg{
		Error: errMsg,
	}, nil
}

func ToSecretRespMsg(responses []*common.ProducedSecretResponse) []*generated.SecretResponseMsg {
	respMsgs := make([]*generated.SecretResponseMsg, len(responses))

	for i, resp := range responses {
		msg := generated.SecretResponseMsg{
			Secret:      resp.Secret,
			RequesterID: resp.RequesterID.Bytes(),
			HostAddress: resp.HostAddress,
		}
		respMsgs[i] = &msg
	}

	return respMsgs
}

func FromSecretRespMsg(secretResponses []*generated.SecretResponseMsg) []*common.ProducedSecretResponse {
	respList := make([]*common.ProducedSecretResponse, len(secretResponses))

	for i, msgResp := range secretResponses {
		r := common.ProducedSecretResponse{
			Secret:      msgResp.Secret,
			RequesterID: gethcommon.BytesToAddress(msgResp.RequesterID),
			HostAddress: msgResp.HostAddress,
		}
		respList[i] = &r
	}
	return respList
}

func FromBlockSubmissionResponseMsg(msg *generated.BlockSubmissionResponseMsg) (*common.BlockSubmissionResponse, error) {
	var subscribedLogs map[rpc.ID][]byte
	if err := json.Unmarshal(msg.SubscribedLogs, &subscribedLogs); err != nil {
		return nil, fmt.Errorf("could not unmarshal subscribed logs from JSON. Cause: %w", err)
	}

	return &common.BlockSubmissionResponse{
		BlockHeader:             FromBlockHeaderMsg(msg.GetBlockHeader()),
		ProducedRollup:          FromExtRollupMsg(msg.ProducedRollup),
		FoundNewHead:            msg.IngestedNewRollup,
		RollupHead:              FromRollupHeaderMsg(msg.RollupHead),
		SubscribedLogs:          subscribedLogs,
		ProducedSecretResponses: FromSecretRespMsg(msg.ProducedSecretResponses),
	}, nil
}

func ToExtRollupMsg(rollup *common.ExtRollup) generated.ExtRollupMsg {
	txHashBytes := make([][]byte, len(rollup.TxHashes))
	for idx, txHash := range rollup.TxHashes {
		txHashBytes[idx] = txHash.Bytes()
	}

	if rollup.Header != nil {
		return generated.ExtRollupMsg{Header: ToRollupHeaderMsg(rollup.Header), TxHashes: txHashBytes, Txs: rollup.EncryptedTxBlob}
	}

	return generated.ExtRollupMsg{Header: nil}
}

func ToRollupHeaderMsg(header *common.Header) *generated.HeaderMsg {
	if header == nil {
		return nil
	}
	var headerMsg generated.HeaderMsg
	withdrawalMsgs := make([]*generated.WithdrawalMsg, 0)
	for _, withdrawal := range header.Withdrawals {
		withdrawalMsg := generated.WithdrawalMsg{Amount: withdrawal.Amount.Bytes(), Recipient: withdrawal.Recipient.Bytes(), Contract: withdrawal.Contract.Bytes()}
		withdrawalMsgs = append(withdrawalMsgs, &withdrawalMsg)
	}

	diff := uint64(0)
	if header.Difficulty != nil {
		diff = header.Difficulty.Uint64()
	}
	baseFee := uint64(0)
	if header.BaseFee != nil {
		baseFee = header.BaseFee.Uint64()
	}
	headerMsg = generated.HeaderMsg{
		ParentHash:  header.ParentHash.Bytes(),
		Node:        header.Agg.Bytes(),
		Nonce:       []byte{},
		Proof:       header.L1Proof.Bytes(),
		Root:        header.Root.Bytes(),
		TxHash:      header.TxHash.Bytes(),
		Number:      header.Number.Uint64(),
		Bloom:       header.Bloom.Bytes(),
		ReceiptHash: header.ReceiptHash.Bytes(),
		Extra:       header.Extra,
		R:           header.R.Bytes(),
		S:           header.S.Bytes(),
		Withdrawals: withdrawalMsgs,
		UncleHash:   header.UncleHash.Bytes(),
		Coinbase:    header.Coinbase.Bytes(),
		Difficulty:  diff,
		GasLimit:    header.GasLimit,
		GasUsed:     header.GasUsed,
		Time:        header.Time,
		MixDigest:   header.MixDigest.Bytes(),
		BaseFee:     baseFee,
	}

	return &headerMsg
}

func FromExtRollupMsg(msg *generated.ExtRollupMsg) common.ExtRollup {
	if msg.Header == nil {
		return common.ExtRollup{
			Header: nil,
		}
	}

	// We recreate the transaction hashes.
	txHashes := make([]gethcommon.Hash, len(msg.TxHashes))
	for idx, bytes := range msg.TxHashes {
		txHashes[idx] = gethcommon.BytesToHash(bytes)
	}

	return common.ExtRollup{
		Header:          FromRollupHeaderMsg(msg.Header),
		TxHashes:        txHashes,
		EncryptedTxBlob: msg.Txs,
	}
}

func FromRollupHeaderMsg(header *generated.HeaderMsg) *common.Header {
	if header == nil {
		return nil
	}
	withdrawals := make([]common.Withdrawal, 0)
	for _, withdrawalMsg := range header.Withdrawals {
		recipient := gethcommon.BytesToAddress(withdrawalMsg.Recipient)
		contract := gethcommon.BytesToAddress(withdrawalMsg.Contract)
		amount := big.NewInt(0).SetBytes(withdrawalMsg.Amount)
		withdrawal := common.Withdrawal{Amount: amount, Recipient: recipient, Contract: contract}
		withdrawals = append(withdrawals, withdrawal)
	}

	r := &big.Int{}
	s := &big.Int{}
	return &common.Header{
		ParentHash:  gethcommon.BytesToHash(header.ParentHash),
		Agg:         gethcommon.BytesToAddress(header.Node),
		Nonce:       types.EncodeNonce(big.NewInt(0).SetBytes(header.Nonce).Uint64()),
		L1Proof:     gethcommon.BytesToHash(header.Proof),
		Root:        gethcommon.BytesToHash(header.Root),
		TxHash:      gethcommon.BytesToHash(header.TxHash),
		Number:      big.NewInt(int64(header.Number)),
		Bloom:       types.BytesToBloom(header.Bloom),
		ReceiptHash: gethcommon.BytesToHash(header.ReceiptHash),
		Extra:       header.Extra,
		R:           r.SetBytes(header.R),
		S:           s.SetBytes(header.S),
		Withdrawals: withdrawals,
		UncleHash:   gethcommon.BytesToHash(header.UncleHash),
		Coinbase:    gethcommon.BytesToAddress(header.Coinbase),
		Difficulty:  big.NewInt(int64(header.Difficulty)),
		GasLimit:    header.GasLimit,
		GasUsed:     header.GasUsed,
		Time:        header.Time,
		MixDigest:   gethcommon.BytesToHash(header.MixDigest),
		BaseFee:     big.NewInt(int64(header.BaseFee)),
	}
}

func FromBlockHeaderMsg(msg *generated.BlockHeaderMsg) *types.Header {
	if msg == nil {
		return nil
	}
	return &types.Header{
		ParentHash:  gethcommon.BytesToHash(msg.ParentHash),
		UncleHash:   gethcommon.BytesToHash(msg.UncleHash),
		Coinbase:    gethcommon.BytesToAddress(msg.Coinbase),
		Root:        gethcommon.BytesToHash(msg.Root),
		TxHash:      gethcommon.BytesToHash(msg.TxHash),
		ReceiptHash: gethcommon.BytesToHash(msg.ReceiptHash),
		Bloom:       types.BytesToBloom(msg.Bloom),
		Difficulty:  big.NewInt(int64(msg.Difficulty)),
		Number:      big.NewInt(int64(msg.Number)),
		GasLimit:    msg.GasLimit,
		GasUsed:     msg.GasUsed,
		Time:        msg.Time,
		Extra:       msg.Extra,
		MixDigest:   gethcommon.BytesToHash(msg.MixDigest),
		Nonce:       types.EncodeNonce(msg.Nonce),
		BaseFee:     big.NewInt(int64(msg.BaseFee)),
	}
}

func ToBlockHeaderMsg(header *types.Header) *generated.BlockHeaderMsg {
	if header == nil {
		return nil
	}
	baseFee := uint64(0)
	if header.BaseFee != nil {
		baseFee = header.BaseFee.Uint64()
	}
	return &generated.BlockHeaderMsg{
		ParentHash:  header.ParentHash.Bytes(),
		UncleHash:   header.UncleHash.Bytes(),
		Coinbase:    header.Coinbase.Bytes(),
		Root:        header.Root.Bytes(),
		TxHash:      header.TxHash.Bytes(),
		ReceiptHash: header.ReceiptHash.Bytes(),
		Bloom:       header.Bloom.Bytes(),
		Difficulty:  header.Difficulty.Uint64(),
		Number:      header.Number.Uint64(),
		GasLimit:    header.GasLimit,
		GasUsed:     header.GasUsed,
		Time:        header.Time,
		Extra:       header.Extra,
		MixDigest:   header.MixDigest.Bytes(),
		Nonce:       header.Nonce.Uint64(),
		BaseFee:     baseFee,
	}
}
