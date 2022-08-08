package erc20contract

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/obscuronet/go-obscuro/integration/erc20contract/generated/ObsERC20"
)

func Bytecode(tokenName string, initialSupply *big.Int) ([]byte, error) {
	parsed, err := ObsERC20.ObsERC20MetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return parsed.Pack("", tokenName, tokenName, initialSupply)
}

func BytecodeWithDefaultSupply(tokenName string) []byte {
	parsed, err := ObsERC20.ObsERC20MetaData.GetAbi()
	if err != nil {
		panic(err)
	}
	initialSupply, _ := big.NewInt(0).SetString("1000000000000000000000000000000000000000", 10)
	input, err := parsed.Pack("", tokenName, tokenName, initialSupply)
	if err != nil {
		panic(err)
	}
	bytecode := common.FromHex(ObsERC20.ObsERC20MetaData.Bin)
	return append(bytecode, input...)
}