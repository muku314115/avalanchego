package network

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	blockexecutor "github.com/ava-labs/avalanchego/vms/platformvm/block/executor"
	"github.com/ava-labs/avalanchego/vms/platformvm/txs"
	"github.com/ava-labs/avalanchego/vms/platformvm/txs/mempool"
)

func TestVerifierMempool(t *testing.T) {

	tests := []struct {
		name      string
		tx        *txs.Tx
		errVerify error
		errAdd    error
	}{
		{
			name: "failed add",
			tx: &txs.Tx{
				Unsigned: &txs.BaseTx{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)
			ctrl := gomock.NewController(t)
			mempool := mempool.NewMockMempool(ctrl)
			mempool.EXPECT().Add(gomock.Any()).Return(tt.errAdd).AnyTimes()
			manager := blockexecutor.NewMockManager(ctrl)
			manager.EXPECT().VerifyTx(gomock.Any()).Return(tt.errVerify).AnyTimes()

			verifierMempool, err := NewVerifierMempool(
				mempool,
				manager,
				txGossipBloomMaxItems,
				txGossipFalsePositiveRate,
				txGossipMaxFalsePositiveRate,
			)

			require.NoError(err)
		})
	}
}
