package clientapi

import (
	"context"
	"fmt"

	"github.com/obscuronet/go-obscuro/go/common/host"

	gethcommon "github.com/ethereum/go-ethereum/common"

	gethlog "github.com/ethereum/go-ethereum/log"
	"github.com/obscuronet/go-obscuro/go/common/log"

	"github.com/obscuronet/go-obscuro/go/common"

	"github.com/ethereum/go-ethereum/rpc"
)

// FilterAPI exposes a subset of Geth's PublicFilterAPI operations.
type FilterAPI struct {
	host   host.Host
	logger gethlog.Logger
}

func NewFilterAPI(host host.Host, logger gethlog.Logger) *FilterAPI {
	return &FilterAPI{
		host:   host,
		logger: logger,
	}
}

// Logs returns a log subscription.
func (api *FilterAPI) Logs(ctx context.Context, encryptedParams common.EncryptedParamsLogSubscription) (*rpc.Subscription, error) {
	notifier, supported := rpc.NotifierFromContext(ctx)
	if !supported {
		return nil, fmt.Errorf("creation of subscriptions is not supported")
	}
	subscription := notifier.CreateSubscription()

	matchedLogs := make(chan []byte)
	err := api.host.Subscribe(subscription.ID, encryptedParams, matchedLogs)
	if err != nil {
		return nil, fmt.Errorf("could not subscribe for logs. Cause: %w", err)
	}

	// We return the ID of the newly-created subscription, before returning any log events.
	err = notifier.Notify(subscription.ID, common.IDAndEncLog{
		SubID: subscription.ID,
	})
	if err != nil {
		api.host.Unsubscribe(subscription.ID)
		return nil, fmt.Errorf("could not send subscription ID to client on subscription %s", subscription.ID)
	}

	go func() {
		for {
			select {
			case encryptedLog := <-matchedLogs:
				idAndEncLog := common.IDAndEncLog{
					SubID:  subscription.ID,
					EncLog: encryptedLog,
				}
				err = notifier.Notify(subscription.ID, idAndEncLog)
				if err != nil {
					api.logger.Error("could not send encrypted log to client on subscription ", log.SubIDKey, subscription.ID)
				}

			case <-subscription.Err(): // client sent an unsubscribe request
				api.host.Unsubscribe(subscription.ID)
				return
			}
		}
	}()

	return subscription, nil
}

// GetLogs returns the logs matching the filter.
func (api *FilterAPI) GetLogs(_ context.Context, encryptedParams common.EncryptedParamsGetLogs) (string, error) {
	encryptedResponse, err := api.host.EnclaveClient().GetLogs(encryptedParams)
	if err != nil {
		return "", err
	}
	if encryptedResponse == nil {
		return "", err
	}
	return gethcommon.Bytes2Hex(encryptedResponse), nil
}
