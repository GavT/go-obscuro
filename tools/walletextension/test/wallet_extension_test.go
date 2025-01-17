package test

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/obscuronet/go-obscuro/go/common"
	wecommon "github.com/obscuronet/go-obscuro/tools/walletextension/common"

	gethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/obscuronet/go-obscuro/tools/walletextension/accountmanager"

	"github.com/obscuronet/go-obscuro/go/rpc"
	"github.com/obscuronet/go-obscuro/tools/walletextension"
)

const (
	errFailedDecrypt = "could not decrypt bytes with viewing key"
	dummyParams      = "dummyParams"
	magicNumber      = 123789
	jsonKeyTopics    = "topics"
)

var dummyHash = gethcommon.BigToHash(big.NewInt(magicNumber))

func TestCanInvokeNonSensitiveMethodsWithoutViewingKey(t *testing.T) {
	createDummyHost(t)
	createWalExt(t, createWalExtCfg())

	respBody, _ := makeWSEthJSONReq(rpc.ChainID, []interface{}{})
	validateJSONResponse(t, respBody)

	if !strings.Contains(string(respBody), l2ChainIDHex) {
		t.Fatalf("expected response containing '%s', got '%s'", l2ChainIDHex, string(respBody))
	}
}

func TestCannotInvokeSensitiveMethodsWithoutViewingKey(t *testing.T) {
	createDummyHost(t)
	createWalExt(t, createWalExtCfg())

	for _, method := range rpc.SensitiveMethods {
		// We use a websocket request because one of the sensitive methods, eth_subscribe, requires it.
		respBody, _ := makeWSEthJSONReq(method, []interface{}{})
		if !strings.Contains(string(respBody), fmt.Sprintf(accountmanager.ErrNoViewingKey, method)) {
			t.Fatalf("expected response containing '%s', got '%s'", fmt.Sprintf(accountmanager.ErrNoViewingKey, method), string(respBody))
		}
	}
}

func TestCanInvokeSensitiveMethodsWithViewingKey(t *testing.T) {
	createDummyHost(t)
	createWalExt(t, createWalExtCfg())

	_, viewingKeyBytes := registerPrivateKey(t, false)
	dummyAPI.setViewingKey(viewingKeyBytes)

	for _, method := range rpc.SensitiveMethods {
		// Subscriptions have to be tested separately, as they return results differently.
		if method == rpc.Subscribe {
			continue
		}

		respBody := makeHTTPEthJSONReq(method, []interface{}{map[string]interface{}{"params": dummyParams}})
		validateJSONResponse(t, respBody)

		if !strings.Contains(string(respBody), dummyParams) {
			t.Fatalf("expected response containing '%s', got '%s'", dummyParams, string(respBody))
		}
	}
}

func TestCannotInvokeSensitiveMethodsWithViewingKeyForAnotherAccount(t *testing.T) {
	createDummyHost(t)
	createWalExt(t, createWalExtCfg())

	registerPrivateKey(t, false)

	// We set the API to decrypt with a key different to the viewing key we just submitted.
	arbitraryPrivateKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf(fmt.Sprintf("failed to generate private key. Cause: %s", err))
	}
	arbitraryPublicKeyBytesHex := hex.EncodeToString(crypto.CompressPubkey(&arbitraryPrivateKey.PublicKey))
	dummyAPI.setViewingKey([]byte(arbitraryPublicKeyBytesHex))

	for _, method := range rpc.SensitiveMethods {
		// Subscriptions have to be tested separately, as they return results differently.
		if method == rpc.Subscribe {
			continue
		}

		respBody := makeHTTPEthJSONReq(method, []interface{}{map[string]interface{}{}})
		if !strings.Contains(string(respBody), errFailedDecrypt) {
			t.Fatalf("expected response containing '%s', got '%s'", errFailedDecrypt, string(respBody))
		}
	}
}

func TestCanInvokeSensitiveMethodsAfterSubmittingMultipleViewingKeys(t *testing.T) {
	createDummyHost(t)
	createWalExt(t, createWalExtCfg())

	// We submit viewing keys for ten arbitrary accounts.
	var viewingKeys [][]byte
	for i := 0; i < 10; i++ {
		_, viewingKeyBytes := registerPrivateKey(t, false)
		viewingKeys = append(viewingKeys, viewingKeyBytes)
	}

	// We set the API to decrypt with an arbitrary key from the list we just generated.
	arbitraryViewingKey := viewingKeys[len(viewingKeys)/2]
	dummyAPI.setViewingKey(arbitraryViewingKey)

	respBody := makeHTTPEthJSONReq(rpc.GetBalance, []interface{}{map[string]interface{}{"params": dummyParams}})
	validateJSONResponse(t, respBody)

	if !strings.Contains(string(respBody), dummyParams) {
		t.Fatalf("expected response containing '%s', got '%s'", dummyParams, string(respBody))
	}
}

func TestCanCallWithoutSettingFromField(t *testing.T) {
	createDummyHost(t)
	createWalExt(t, createWalExtCfg())

	vkAddress, viewingKeyBytes := registerPrivateKey(t, false)
	dummyAPI.setViewingKey(viewingKeyBytes)

	for _, method := range []string{rpc.Call, rpc.EstimateGas} {
		respBody := makeHTTPEthJSONReq(method, []interface{}{map[string]interface{}{
			"To":    "0xf3a8bd422097bFdd9B3519Eaeb533393a1c561aC",
			"data":  "0x70a0823100000000000000000000000013e23ca74de0206c56ebae8d51b5622eff1e9944",
			"value": nil,
			"Value": "",
		}})
		validateJSONResponse(t, respBody)

		// RPCCall and RPCEstimateGas payload might be manipulated ( added the From field information )
		if !strings.Contains(strings.ToLower(string(respBody)), strings.ToLower(vkAddress.Hex())) {
			t.Fatalf("expected response containing '%s', got '%s'", strings.ToLower(vkAddress.Hex()), string(respBody))
		}
	}
}

func TestKeysAreReloadedWhenWalletExtensionRestarts(t *testing.T) {
	createDummyHost(t)
	walExtCfg := createWalExtCfg()
	shutdown := createWalExt(t, walExtCfg)

	_, viewingKeyBytes := registerPrivateKey(t, false)
	dummyAPI.setViewingKey(viewingKeyBytes)

	// We shut down the wallet extension and restart it with the same config, forcing the viewing keys to be reloaded.
	shutdown()
	createWalExt(t, walExtCfg)

	respBody := makeHTTPEthJSONReq(rpc.GetBalance, []interface{}{map[string]interface{}{"params": dummyParams}})
	validateJSONResponse(t, respBody)

	if !strings.Contains(string(respBody), dummyParams) {
		t.Fatalf("expected response containing '%s', got '%s'", dummyParams, string(respBody))
	}
}

func TestCannotSubscribeOverHTTP(t *testing.T) {
	createDummyHost(t)
	createWalExt(t, createWalExtCfg())

	respBody := makeHTTPEthJSONReq(rpc.Subscribe, []interface{}{rpc.SubscriptionTypeLogs})
	if string(respBody) != walletextension.ErrSubscribeFailHTTP+"\n" {
		t.Fatalf("expected response of '%s', got '%s'", walletextension.ErrSubscribeFailHTTP, string(respBody))
	}
}

func TestCanRegisterViewingKeyAndMakeRequestsOverWebsockets(t *testing.T) {
	createDummyHost(t)
	createWalExt(t, createWalExtCfg())

	_, viewingKeyBytes := registerPrivateKey(t, true)
	dummyAPI.setViewingKey(viewingKeyBytes)

	for _, method := range rpc.SensitiveMethods {
		// Subscriptions have to be tested separately, as they return results differently.
		if method == rpc.Subscribe {
			continue
		}

		respBody, _ := makeWSEthJSONReq(method, []interface{}{map[string]interface{}{"params": dummyParams}})
		validateJSONResponse(t, respBody)

		if !strings.Contains(string(respBody), dummyParams) {
			t.Fatalf("expected response containing '%s', got '%s'", dummyParams, string(respBody))
		}

		return // We only need to test a single sensitive method.
	}
}

func TestCanSubscribeForLogsOverWebsockets(t *testing.T) {
	createDummyHost(t)
	createWalExt(t, createWalExtCfg())

	_, viewingKeyBytes := registerPrivateKey(t, false)
	dummyAPI.setViewingKey(viewingKeyBytes)

	filter := common.FilterCriteriaJSON{Topics: []interface{}{dummyHash}}
	resp, conn := makeWSEthJSONReq(rpc.Subscribe, []interface{}{rpc.SubscriptionTypeLogs, filter})
	validateSubscriptionResponse(t, resp)

	logsJSON := readMessagesForDuration(t, conn, time.Second)

	// We check we received enough logs.
	if len(logsJSON) < 50 {
		t.Errorf("expected to receive at least 50 logs, only received %d", len(logsJSON))
	}

	// We check that none of the logs were duplicates (i.e. were sent twice).
	assertNoDupeLogs(t, logsJSON)

	// We validate that each log contains the correct topic.
	for _, logJSON := range logsJSON {
		var logResp map[string]interface{}
		err := json.Unmarshal(logJSON, &logResp)
		if err != nil {
			t.Fatalf("could not unmarshal received log from JSON")
		}

		// We extract the topic from the received logs. The API should have set this based on the filter we passed when subscribing.
		logMap := logResp[wecommon.JSONKeyParams].(map[string]interface{})[wecommon.JSONKeyResult].(map[string]interface{})
		firstLogTopic := logMap[jsonKeyTopics].([]interface{})[0].(string)

		if firstLogTopic != dummyHash.Hex() {
			t.Errorf("expected first topic to be '%s', got '%s'", dummyHash.Hex(), firstLogTopic)
		}
	}
}
