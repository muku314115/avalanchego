// Copyright (C) 2019-2023, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package network

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"go.uber.org/mock/gomock"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/proto/pb/sdk"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/snow/engine/common"
	"github.com/ava-labs/avalanchego/utils/crypto/secp256k1"
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/ava-labs/avalanchego/vms/components/avax"
	"github.com/ava-labs/avalanchego/vms/components/message"
	"github.com/ava-labs/avalanchego/vms/platformvm/block/executor"
	"github.com/ava-labs/avalanchego/vms/platformvm/txs"
	"github.com/ava-labs/avalanchego/vms/platformvm/txs/mempool"
)

const (
	txGossipThrottlingPeriod     = time.Second
	txGossipThrottlingLimit      = 1
	txGossipBloomMaxItems        = 10
	txGossipFalsePositiveRate    = 0.1
	txGossipMaxFalsePositiveRate = 0.5
)

var (
	errTest               = errors.New("test error")
	errFailedVerification = errors.New("failed verification")

	txGossipHandlerPrefix = binary.AppendUvarint(nil, txGossipHandlerID)
)

func TestNetworkAppGossip(t *testing.T) {
	testTx := &txs.Tx{
		Unsigned: &txs.BaseTx{
			BaseTx: avax.BaseTx{
				NetworkID:    1,
				BlockchainID: ids.GenerateTestID(),
				Ins:          []*avax.TransferableInput{},
				Outs:         []*avax.TransferableOutput{},
			},
		},
	}
	require.NoError(t, testTx.Initialize(txs.Codec))

	type test struct {
		name                      string
		msgBytesFunc              func() []byte
		mempoolFunc               func(*gomock.Controller) mempool.Mempool
		partialSyncPrimaryNetwork bool
		appSenderFunc             func(*gomock.Controller) common.AppSender
	}

	tests := []test{
		{
			// Shouldn't attempt to issue or gossip the tx
			name: "invalid message bytes",
			msgBytesFunc: func() []byte {
				return []byte{0x00}
			},
			mempoolFunc: func(ctrl *gomock.Controller) mempool.Mempool {
				// Unused in this test
				return nil
			},
			appSenderFunc: func(ctrl *gomock.Controller) common.AppSender {
				// Unused in this test
				return nil
			},
		},
		{
			// Shouldn't attempt to issue or gossip the tx
			name: "invalid tx bytes",
			msgBytesFunc: func() []byte {
				msg := message.Tx{
					Tx: []byte{0x00},
				}
				msgBytes, err := message.Build(&msg)
				require.NoError(t, err)
				return msgBytes
			},
			mempoolFunc: func(ctrl *gomock.Controller) mempool.Mempool {
				// Unused in this test
				return mempool.NewMockMempool(ctrl)
			},
			appSenderFunc: func(ctrl *gomock.Controller) common.AppSender {
				// Unused in this test
				return common.NewMockSender(ctrl)
			},
		},
		{
			// Issue returns nil because mempool has tx. We should gossip the tx.
			name: "issuance succeeds",
			msgBytesFunc: func() []byte {
				msg := message.Tx{
					Tx: testTx.Bytes(),
				}
				msgBytes, err := message.Build(&msg)
				require.NoError(t, err)
				return msgBytes
			},
			mempoolFunc: func(ctrl *gomock.Controller) mempool.Mempool {
				mempool := mempool.NewMockMempool(ctrl)
				mempool.EXPECT().Has(gomock.Any()).Return(true)
				mempool.EXPECT().GetDropReason(gomock.Any()).Return(nil)
				return mempool
			},
			appSenderFunc: func(ctrl *gomock.Controller) common.AppSender {
				appSender := common.NewMockSender(ctrl)
				appSender.EXPECT().SendAppGossip(gomock.Any(), gomock.Any()).Times(2)
				return appSender
			},
		},
		{
			// Issue returns error because tx was dropped. We shouldn't gossip the tx.
			name: "issuance fails",
			msgBytesFunc: func() []byte {
				msg := message.Tx{
					Tx: testTx.Bytes(),
				}
				msgBytes, err := message.Build(&msg)
				require.NoError(t, err)
				return msgBytes
			},
			mempoolFunc: func(ctrl *gomock.Controller) mempool.Mempool {
				mempool := mempool.NewMockMempool(ctrl)
				mempool.EXPECT().GetDropReason(gomock.Any()).Return(errTest)
				return mempool
			},
			appSenderFunc: func(ctrl *gomock.Controller) common.AppSender {
				// Unused in this test
				return common.NewMockSender(ctrl)
			},
		},
		{
			name: "should AppGossip if primary network is not being fully synced",
			msgBytesFunc: func() []byte {
				msg := message.Tx{
					Tx: testTx.Bytes(),
				}
				msgBytes, err := message.Build(&msg)
				require.NoError(t, err)
				return msgBytes
			},
			mempoolFunc: func(ctrl *gomock.Controller) mempool.Mempool {
				mempool := mempool.NewMockMempool(ctrl)
				// mempool.EXPECT().Has(gomock.Any()).Return(true)
				return mempool
			},
			partialSyncPrimaryNetwork: true,
			appSenderFunc: func(ctrl *gomock.Controller) common.AppSender {
				appSender := common.NewMockSender(ctrl)
				// appSender.EXPECT().SendAppGossip(gomock.Any(), gomock.Any())
				return appSender
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctrl := gomock.NewController(t)

			n, err := New(
				&snow.Context{
					Log: logging.NoLog{},
				},
				executor.NewMockManager(ctrl), // Manager is unused in this test
				tt.mempoolFunc(ctrl),
				tt.partialSyncPrimaryNetwork,
				tt.appSenderFunc(ctrl),
				txGossipThrottlingPeriod,
				txGossipThrottlingLimit,
				txGossipBloomMaxItems,
				txGossipFalsePositiveRate,
				txGossipMaxFalsePositiveRate,
				prometheus.NewRegistry(),
			)
			require.NoError(err)
			require.NoError(n.AppGossip(ctx, ids.GenerateTestNodeID(), tt.msgBytesFunc()))
		})
	}
}

func TestNetworkIssueTx(t *testing.T) {
	type test struct {
		name                      string
		mempoolFunc               func(*gomock.Controller) mempool.Mempool
		managerFunc               func(*gomock.Controller) executor.Manager
		partialSyncPrimaryNetwork bool
		appSenderFunc             func(*gomock.Controller) common.AppSender
		expectedErr               error
	}

	tests := []test{
		{
			name: "mempool has transaction",
			mempoolFunc: func(ctrl *gomock.Controller) mempool.Mempool {
				mempool := mempool.NewMockMempool(ctrl)
				mempool.EXPECT().Has(gomock.Any()).Return(true)
				return mempool
			},
			managerFunc: func(ctrl *gomock.Controller) executor.Manager {
				// Unused in this test
				return executor.NewMockManager(ctrl)
			},
			appSenderFunc: func(ctrl *gomock.Controller) common.AppSender {
				// Should gossip the tx
				appSender := common.NewMockSender(ctrl)
				appSender.EXPECT().SendAppGossip(gomock.Any(), gomock.Any()).Return(nil).Times(2)
				return appSender
			},
			expectedErr: nil,
		},
		{
			name: "transaction invalid",
			mempoolFunc: func(ctrl *gomock.Controller) mempool.Mempool {
				mempool := mempool.NewMockMempool(ctrl)
				mempool.EXPECT().Has(gomock.Any()).Return(false)
				mempool.EXPECT().MarkDropped(gomock.Any(), gomock.Any())
				return mempool
			},
			managerFunc: func(ctrl *gomock.Controller) executor.Manager {
				manager := executor.NewMockManager(ctrl)
				manager.EXPECT().VerifyTx(gomock.Any()).Return(errTest)
				return manager
			},
			appSenderFunc: func(ctrl *gomock.Controller) common.AppSender {
				// Shouldn't gossip the tx
				return common.NewMockSender(ctrl)
			},
			expectedErr: errTest,
		},
		{
			name: "can't add transaction to mempool",
			mempoolFunc: func(ctrl *gomock.Controller) mempool.Mempool {
				mempool := mempool.NewMockMempool(ctrl)
				mempool.EXPECT().Has(gomock.Any()).Return(false)
				mempool.EXPECT().Add(gomock.Any()).Return(errTest)
				mempool.EXPECT().MarkDropped(gomock.Any(), errTest)
				return mempool
			},
			managerFunc: func(ctrl *gomock.Controller) executor.Manager {
				manager := executor.NewMockManager(ctrl)
				manager.EXPECT().VerifyTx(gomock.Any()).Return(nil)
				return manager
			},
			appSenderFunc: func(ctrl *gomock.Controller) common.AppSender {
				// Shouldn't gossip the tx
				return common.NewMockSender(ctrl)
			},
			expectedErr: errTest,
		},
		{
			name: "AppGossip tx but do not add to mempool if primary network is not being fully synced",
			mempoolFunc: func(ctrl *gomock.Controller) mempool.Mempool {
				return mempool.NewMockMempool(ctrl)
			},
			managerFunc: func(ctrl *gomock.Controller) executor.Manager {
				return executor.NewMockManager(ctrl)
			},
			partialSyncPrimaryNetwork: true,
			appSenderFunc: func(ctrl *gomock.Controller) common.AppSender {
				// Should gossip the tx
				appSender := common.NewMockSender(ctrl)
				appSender.EXPECT().SendAppGossip(gomock.Any(), gomock.Any()).Times(2)
				return appSender
			},
			expectedErr: nil,
		},
		{
			name: "happy path",
			mempoolFunc: func(ctrl *gomock.Controller) mempool.Mempool {
				mempool := mempool.NewMockMempool(ctrl)
				mempool.EXPECT().Has(gomock.Any()).Return(false)
				mempool.EXPECT().Add(gomock.Any()).Return(nil)
				mempool.EXPECT().RequestBuildBlock(false)
				return mempool
			},
			managerFunc: func(ctrl *gomock.Controller) executor.Manager {
				manager := executor.NewMockManager(ctrl)
				manager.EXPECT().VerifyTx(gomock.Any()).Return(nil)
				return manager
			},
			appSenderFunc: func(ctrl *gomock.Controller) common.AppSender {
				// Should gossip the tx
				appSender := common.NewMockSender(ctrl)
				appSender.EXPECT().SendAppGossip(gomock.Any(), gomock.Any()).Times(2)
				return appSender
			},
			expectedErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)
			ctrl := gomock.NewController(t)

			n, err := New(
				&snow.Context{
					Log: logging.NoLog{},
				},
				tt.managerFunc(ctrl),
				tt.mempoolFunc(ctrl),
				tt.partialSyncPrimaryNetwork,
				tt.appSenderFunc(ctrl),
				txGossipThrottlingPeriod,
				txGossipThrottlingLimit,
				txGossipBloomMaxItems,
				txGossipFalsePositiveRate,
				txGossipMaxFalsePositiveRate,
				prometheus.NewRegistry(),
			)
			require.NoError(err)
			err = n.IssueTx(context.Background(), &txs.Tx{})
			require.ErrorIs(err, tt.expectedErr)
		})
	}
}

func TestNetworkGossipTx(t *testing.T) {
	require := require.New(t)
	ctrl := gomock.NewController(t)

	appSender := common.NewMockSender(ctrl)

	n, err := New(
		&snow.Context{
			Log: logging.NoLog{},
		},
		executor.NewMockManager(ctrl),
		mempool.NewMockMempool(ctrl),
		false,
		appSender,
		txGossipThrottlingPeriod,
		txGossipThrottlingLimit,
		txGossipBloomMaxItems,
		txGossipFalsePositiveRate,
		txGossipMaxFalsePositiveRate,
		prometheus.NewRegistry(),
	)
	require.NoError(err)
	require.IsType(&Network{}, n)

	// Case: Tx was recently gossiped
	txID := ids.GenerateTestID()
	n.recentTxs.Put(txID, struct{}{})
	n.legacyGossipTx(context.Background(), txID, []byte{})
	// Didn't make a call to SendAppGossip

	// Case: Tx was not recently gossiped
	msgBytes := []byte{1, 2, 3}
	appSender.EXPECT().SendAppGossip(gomock.Any(), msgBytes).Return(nil)
	n.legacyGossipTx(context.Background(), ids.GenerateTestID(), msgBytes)
	// Did make a call to SendAppGossip
}

func TestNetworkInboundTxPushGossip(t *testing.T) {
	require := require.New(t)
	ctrl := gomock.NewController(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	snowCtx := snow.DefaultContextTest()

	mockVerifier := executor.NewMockManager(ctrl)
	mockMempool := mempool.NewMockMempool(ctrl)

	sender := &common.FakeSender{
		SentAppGossip: make(chan []byte, 1),
	}
	network, err := New(
		snowCtx,
		mockVerifier,
		mockMempool,
		false,
		sender,
		txGossipThrottlingPeriod,
		txGossipThrottlingLimit,
		txGossipBloomMaxItems,
		txGossipFalsePositiveRate,
		txGossipMaxFalsePositiveRate,
		prometheus.NewRegistry(),
	)

	utx := &txs.RewardValidatorTx{
		TxID: ids.GenerateTestID(),
	}

	pk, err := secp256k1.NewPrivateKey()
	require.NoError(err)
	tx, err := txs.NewSigned(utx, txs.Codec, [][]*secp256k1.PrivateKey{{pk}})
	require.NoError(err)

	txBytes, err := tx.Marshal()
	require.NoError(err)

	inboundGossip := &sdk.PushGossip{
		Gossip: [][]byte{txBytes},
	}
	inboundGossipBytes, err := proto.Marshal(inboundGossip)
	require.NoError(err)
	inboundMsgBytes := append(txGossipHandlerPrefix, inboundGossipBytes...)

	mockVerifier.EXPECT().VerifyTx(tx).Return(nil)
	mockMempool.EXPECT().Add(tx).Return(nil)

	require.NoError(network.AppGossip(ctx, ids.EmptyNodeID, inboundMsgBytes))

	forwardedMsgBytes := <-sender.SentAppGossip
	forwardedMsg := &sdk.PushGossip{}

	require.Equal(byte(txGossipHandlerID), forwardedMsgBytes[0])
	require.NoError(proto.Unmarshal(forwardedMsgBytes[1:], forwardedMsg))
	require.Len(forwardedMsg.Gossip, 1)

	forwardedTx := &txs.Tx{}
	require.NoError(forwardedTx.Unmarshal(forwardedMsg.Gossip[0]))
	require.Equal(tx.ID(), forwardedTx.ID())
}

func TestNetworkOutboundTxPushGossip(t *testing.T) {
	require := require.New(t)
	ctrl := gomock.NewController(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	snowCtx := snow.DefaultContextTest()

	mockVerifier := executor.NewMockManager(ctrl)
	mockMempool := mempool.NewMockMempool(ctrl)

	sender := &common.FakeSender{
		SentAppGossip: make(chan []byte, 1),
	}
	network, err := New(
		snowCtx,
		mockVerifier,
		mockMempool,
		false,
		sender,
		txGossipThrottlingPeriod,
		txGossipThrottlingLimit,
		txGossipBloomMaxItems,
		txGossipFalsePositiveRate,
		txGossipMaxFalsePositiveRate,
		prometheus.NewRegistry(),
	)

	utx := &txs.RewardValidatorTx{
		TxID: ids.GenerateTestID(),
	}

	pk, err := secp256k1.NewPrivateKey()
	require.NoError(err)
	tx, err := txs.NewSigned(utx, txs.Codec, [][]*secp256k1.PrivateKey{{pk}})
	require.NoError(err)

	txBytes, err := tx.Marshal()
	require.NoError(err)

	inboundGossip := &sdk.PushGossip{
		Gossip: [][]byte{txBytes},
	}
	inboundGossipBytes, err := proto.Marshal(inboundGossip)
	require.NoError(err)
	inboundMsgBytes := append(txGossipHandlerPrefix, inboundGossipBytes...)

	mockVerifier.EXPECT().VerifyTx(tx).Return(nil)
	mockMempool.EXPECT().Add(tx).Return(nil)

	require.NoError(network.AppGossip(ctx, ids.EmptyNodeID, inboundMsgBytes))

	forwardedMsgBytes := <-sender.SentAppGossip
	forwardedMsg := &sdk.PushGossip{}

	require.Equal(byte(txGossipHandlerID), forwardedMsgBytes[0])
	require.NoError(proto.Unmarshal(forwardedMsgBytes[1:], forwardedMsg))
	require.Len(forwardedMsg.Gossip, 1)

	forwardedTx := &txs.Tx{}
	require.NoError(forwardedTx.Unmarshal(forwardedMsg.Gossip[0]))
	require.Equal(tx.ID(), forwardedTx.ID())
}
