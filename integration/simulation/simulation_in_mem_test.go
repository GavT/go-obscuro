package simulation

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"

	"github.com/ethereum/go-ethereum/common"

	"github.com/ethereum/go-ethereum/core/types"

	"github.com/obscuronet/obscuro-playground/go/buildhelper"

	"github.com/obscuronet/obscuro-playground/go/buildhelper/helpertypes"

	"github.com/obscuronet/obscuro-playground/integration/simulation/params"

	"github.com/obscuronet/obscuro-playground/integration/simulation/network"
)

// This test creates a network of in memory L1 and L2 nodes, then injects transactions, and finally checks the resulting output blockchain.
// Running it long enough with various parameters will test many corner cases without having to explicitly write individual tests for them.
// The unit of time is the "AvgBlockDuration" - which is the average time between L1 blocks, which are the carriers of rollups.
// Everything else is reported to this value. This number has to be adjusted in conjunction with the number of nodes. If it's too low,
// the CPU usage will be very high during the simulation which might result in inconclusive results.
func TestInMemoryMonteCarloSimulation(t *testing.T) {
	setupTestLog()

	params := params.SimParams{
		NumberOfNodes:             10,
		NumberOfWallets:           5,
		AvgBlockDuration:          40 * time.Microsecond,
		SimulationTime:            15 * time.Second,
		L1EfficiencyThreshold:     0.2,
		L2EfficiencyThreshold:     0.32,
		L2ToL1EfficiencyThreshold: 0.5,
	}

	params.AvgNetworkLatency = params.AvgBlockDuration / 15
	params.AvgGossipPeriod = params.AvgBlockDuration / 3

	testSimulation(t, network.NewBasicNetworkOfInMemoryNodes(), params)
}

// TestMemObscuroRealEthMonteCarloSimulation runs the simulation against a ganache network
// 1. Install ganache -> npm install ganache --global
// 2. Run ganache -> rm -rf ganachedb &&  ganache --database.dbPath="./ganachedb"  -l 1024000000000 --wallet.accounts="0x5dbbff1b5ff19f1ad6ea656433be35f6846e890b3f3ec6ef2b2e2137a8cab4ae,0x56BC75E2D63100000" --wallet.accounts="0xb728cd9a9f54cede03a82fc189eab4830a612703d48b7ef43ceed2cbad1a06c7,0x56BC75E2D63100000" --wallet.accounts="0x1e1e76d5c0ea1382b6acf76e873977fd223c7fa2a6dc57db2b94e93eb303ba85,0x56BC75E2D63100000" -p 7545 -g 225
func TestMemObscuroRealEthMonteCarloSimulation(t *testing.T) {
	t.Skip("test under construction")
	setupTestLog()
	deployContract(t)

	helpertypes.IsRealEth = true
	params := params.SimParams{
		NumberOfNodes:             2,
		NumberOfWallets:           2,
		AvgBlockDuration:          10 * time.Millisecond,
		SimulationTime:            15 * time.Second,
		L1EfficiencyThreshold:     0.9, // todo review this
		L2EfficiencyThreshold:     0.9,
		L2ToL1EfficiencyThreshold: 0.9,
	}

	params.AvgNetworkLatency = params.AvgBlockDuration / 15
	params.AvgGossipPeriod = params.AvgBlockDuration / 2

	testSimulation(t, network.NewNetworkInMemoryGeth(), params)
}

func deployContract(t *testing.T) {
	contractByte := `608060405234801561001057600080fd5b50610cc2806100206000396000f3fe60806040526004361061007b5760003560e01c80638ce0bd461161004e5780638ce0bd4614610128578063b2cd44a014610144578063e0643dfc1461016d578063fc7e286d146101aa5761007b565b806347f03a84146100805780635c26b626146100ab57806380b8c155146100d45780638353ffca146100ff575b600080fd5b34801561008c57600080fd5b506100956101e7565b6040516100a291906109ac565b60405180910390f35b3480156100b757600080fd5b506100d260048036038101906100cd919061078c565b6102d0565b005b3480156100e057600080fd5b506100e9610322565b6040516100f691906109ce565b60405180910390f35b34801561010b57600080fd5b5061012660048036038101906101219190610822565b6103b4565b005b610142600480360381019061013d919061075f565b6103ff565b005b34801561015057600080fd5b5061016b600480360381019061016691906107d9565b610446565b005b34801561017957600080fd5b50610194600480360381019061018f9190610862565b610460565b6040516101a191906109ce565b60405180910390f35b3480156101b657600080fd5b506101d160048036038101906101cc919061075f565b610519565b6040516101de91906109f0565b60405180910390f35b6060600080438152602001908152602001600020805480602002602001604051908101604052809291908181526020016000905b828210156102c757838290600052602060002001805461023a90610b57565b80601f016020809104026020016040519081016040528092919081815260200182805461026690610b57565b80156102b35780601f10610288576101008083540402835291602001916102b3565b820191906000526020600020905b81548152906001019060200180831161029657829003601f168201915b50505050508152602001906001019061021b565b50505050905090565b60008043815260200190815260200160002082829091806001815401808255809150506001900390600052602060002001600090919290919290919290919250919061031d929190610531565b505050565b60606002805461033190610b57565b80601f016020809104026020016040519081016040528092919081815260200182805461035d90610b57565b80156103aa5780601f1061037f576101008083540402835291602001916103aa565b820191906000526020600020905b81548152906001019060200180831161038d57829003601f168201915b5050505050905090565b8073ffffffffffffffffffffffffffffffffffffffff166108fc839081150290604051600060405180830381858888f193505050501580156103fa573d6000803e3d6000fd5b505050565b34600160008373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020019081526020016000208190555050565b806002908051906020019061045c9291906105b7565b5050565b6000602052816000526040600020818154811061047c57600080fd5b9060005260206000200160009150915050805461049890610b57565b80601f01602080910402602001604051908101604052809291908181526020018280546104c490610b57565b80156105115780601f106104e657610100808354040283529160200191610511565b820191906000526020600020905b8154815290600101906020018083116104f457829003601f168201915b505050505081565b60016020528060005260406000206000915090505481565b82805461053d90610b57565b90600052602060002090601f01602090048101928261055f57600085556105a6565b82601f1061057857803560ff19168380011785556105a6565b828001600101855582156105a6579182015b828111156105a557823582559160200191906001019061058a565b5b5090506105b3919061063d565b5090565b8280546105c390610b57565b90600052602060002090601f0160209004810192826105e5576000855561062c565b82601f106105fe57805160ff191683800117855561062c565b8280016001018555821561062c579182015b8281111561062b578251825591602001919060010190610610565b5b509050610639919061063d565b5090565b5b8082111561065657600081600090555060010161063e565b5090565b600061066d61066884610a30565b610a0b565b90508281526020810184848401111561068957610688610c27565b5b610694848285610b15565b509392505050565b6000813590506106ab81610c47565b92915050565b6000813590506106c081610c5e565b92915050565b60008083601f8401126106dc576106db610c1d565b5b8235905067ffffffffffffffff8111156106f9576106f8610c18565b5b60208301915083600182028301111561071557610714610c22565b5b9250929050565b600082601f83011261073157610730610c1d565b5b813561074184826020860161065a565b91505092915050565b60008135905061075981610c75565b92915050565b60006020828403121561077557610774610c31565b5b60006107838482850161069c565b91505092915050565b600080602083850312156107a3576107a2610c31565b5b600083013567ffffffffffffffff8111156107c1576107c0610c2c565b5b6107cd858286016106c6565b92509250509250929050565b6000602082840312156107ef576107ee610c31565b5b600082013567ffffffffffffffff81111561080d5761080c610c2c565b5b6108198482850161071c565b91505092915050565b6000806040838503121561083957610838610c31565b5b60006108478582860161074a565b9250506020610858858286016106b1565b9150509250929050565b6000806040838503121561087957610878610c31565b5b60006108878582860161074a565b92505060206108988582860161074a565b9150509250929050565b60006108ae838361092b565b905092915050565b60006108c182610a71565b6108cb8185610a94565b9350836020820285016108dd85610a61565b8060005b8581101561091957848403895281516108fa85826108a2565b945061090583610a87565b925060208a019950506001810190506108e1565b50829750879550505050505092915050565b600061093682610a7c565b6109408185610aa5565b9350610950818560208601610b24565b61095981610c36565b840191505092915050565b600061096f82610a7c565b6109798185610ab6565b9350610989818560208601610b24565b61099281610c36565b840191505092915050565b6109a681610b0b565b82525050565b600060208201905081810360008301526109c681846108b6565b905092915050565b600060208201905081810360008301526109e88184610964565b905092915050565b6000602082019050610a05600083018461099d565b92915050565b6000610a15610a26565b9050610a218282610b89565b919050565b6000604051905090565b600067ffffffffffffffff821115610a4b57610a4a610be9565b5b610a5482610c36565b9050602081019050919050565b6000819050602082019050919050565b600081519050919050565b600081519050919050565b6000602082019050919050565b600082825260208201905092915050565b600082825260208201905092915050565b600082825260208201905092915050565b6000610ad282610aeb565b9050919050565b6000610ae482610aeb565b9050919050565b600073ffffffffffffffffffffffffffffffffffffffff82169050919050565b6000819050919050565b82818337600083830152505050565b60005b83811015610b42578082015181840152602081019050610b27565b83811115610b51576000848401525b50505050565b60006002820490506001821680610b6f57607f821691505b60208210811415610b8357610b82610bba565b5b50919050565b610b9282610c36565b810181811067ffffffffffffffff82111715610bb157610bb0610be9565b5b80604052505050565b7f4e487b7100000000000000000000000000000000000000000000000000000000600052602260045260246000fd5b7f4e487b7100000000000000000000000000000000000000000000000000000000600052604160045260246000fd5b600080fd5b600080fd5b600080fd5b600080fd5b600080fd5b600080fd5b6000601f19601f8301169050919050565b610c5081610ac7565b8114610c5b57600080fd5b50565b610c6781610ad9565b8114610c7257600080fd5b50565b610c7e81610b0b565b8114610c8957600080fd5b5056fea2646970667358221220382f61b400ade4111e9bd34b846340e65f0eee85ffc7ec3ea73d057688847afd64736f6c63430008070033`

	tmpClient := buildhelper.NewEthAPI("127.0.0.1", 7545)
	err := tmpClient.Connect()
	if err != nil {
		panic(err)
	}

	deployContractTx := types.LegacyTx{
		Nonce:    0, // relies on a clean env
		GasPrice: big.NewInt(2000000000),
		Gas:      1025_000_000,
		Data:     common.Hex2Bytes(contractByte),
	}

	signedTx, err := types.SignNewTx(buildhelper.Addr1PK(), types.NewEIP155Signer(big.NewInt(1337)), &deployContractTx)
	if err != nil {
		t.Fatal(err)
	}

	err = tmpClient.ApiClient.SendTransaction(context.Background(), signedTx)
	if err != nil {
		t.Fatal(err)
	}

	var receipt *types.Receipt
	for start := time.Now(); time.Since(start) < 10*time.Second; time.Sleep(time.Second) {
		receipt, err = tmpClient.ApiClient.TransactionReceipt(context.Background(), signedTx.Hash())
		if err == nil {
			break
		}
		if err != ethereum.NotFound {
			t.Fatal(err)
		}
		t.Logf("Contract deploy tx has not been mined into a block after %s...", time.Since(start))
	}

	t.Logf("Contract deployed to %s\n", receipt.ContractAddress)
}
