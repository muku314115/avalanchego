// Copyright (C) 2019-2023, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package network

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"
	"time"

	bloomfilter "github.com/holiman/bloomfilter/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"go.uber.org/mock/gomock"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/network/p2p/gossip"
	"github.com/ava-labs/avalanchego/proto/pb/sdk"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/snow/engine/common"
	"github.com/ava-labs/avalanchego/snow/validators"
	"github.com/ava-labs/avalanchego/utils/crypto/secp256k1"
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/ava-labs/avalanchego/vms/components/avax"
	"github.com/ava-labs/avalanchego/vms/components/message"
	"github.com/ava-labs/avalanchego/vms/platformvm/block/executor"
	"github.com/ava-labs/avalanchego/vms/platformvm/txs"
	"github.com/ava-labs/avalanchego/vms/platformvm/txs/mempool"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
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

	tx, err := testTx(1)
	require.NoError(err)

	txBytes, err := tx.Marshal()
	require.NoError(err)

	inboundGossip := &sdk.PushGossip{
		Gossip: [][]byte{txBytes},
	}
	inboundGossipBytes, err := proto.Marshal(inboundGossip)
	require.NoError(err)
	inboundMsgBytes := append(txGossipHandlerPrefix, inboundGossipBytes...)

	mockVerifier.EXPECT().VerifyTx(gomock.Any()).Return(nil)
	mockMempool.EXPECT().Add(gomock.Any()).Return(nil)

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
		SentAppGossip: make(chan []byte, 2),
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

	tx, err := testTx(1)
	require.NoError(err)

	mockVerifier.EXPECT().VerifyTx(tx).Return(nil)
	mockMempool.EXPECT().Has(tx.ID()).Return(false)
	mockMempool.EXPECT().Add(tx).Return(nil)
	mockMempool.EXPECT().RequestBuildBlock(gomock.Any())

	require.NoError(network.IssueTx(ctx, tx))

	<-sender.SentAppGossip // legacy outbound gossip message
	gossipMsgBytes := <-sender.SentAppGossip
	gossipMsg := &sdk.PushGossip{}

	require.Equal(byte(txGossipHandlerID), gossipMsgBytes[0])
	require.NoError(proto.Unmarshal(gossipMsgBytes[1:], gossipMsg))
	require.Len(gossipMsg.Gossip, 1)

	gossipTx := &txs.Tx{}
	require.NoError(gossipTx.Unmarshal(gossipMsg.Gossip[0]))
	require.Equal(tx.ID(), gossipTx.ID())
}

func TestNetworkMakesOutboundPullGossipRequests(t *testing.T) {
	require := require.New(t)
	ctrl := gomock.NewController(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	snowCtx := snow.DefaultContextTest()
	snowCtx.NodeID = ids.GenerateTestNodeID()
	// we must be a validator to request gossip
	snowCtx.ValidatorState = &validators.TestState{
		GetCurrentHeightF: func(context.Context) (uint64, error) {
			return 0, nil
		},
		GetValidatorSetF: func(context.Context, uint64, ids.ID) (map[ids.NodeID]*validators.GetValidatorOutput, error) {
			return map[ids.NodeID]*validators.GetValidatorOutput{
				snowCtx.NodeID: nil,
			}, nil
		},
	}

	mockVerifier := executor.NewMockManager(ctrl)
	mockVerifier.EXPECT().VerifyTx(gomock.Any()).Return(nil)
	mockMempool := mempool.NewMockMempool(ctrl)
	mockMempool.EXPECT().Add(gomock.Any()).Return(nil)

	sender := &common.FakeSender{
		SentAppRequest: make(chan []byte, 1),
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
	require.NoError(network.Connected(ctx, snowCtx.NodeID, nil))

	tx1, err := testTx(1)
	require.NoError(err)
	require.NoError(network.mempool.Add(tx1))

	go network.Gossip(ctx)

	pullGossipRequestBytes := <-sender.SentAppRequest
	pullGossipRequest := &sdk.PullGossipRequest{}
	require.Equal(byte(txGossipHandlerID), pullGossipRequestBytes[0])
	require.NoError(proto.Unmarshal(pullGossipRequestBytes[1:], pullGossipRequest))

	bloomFilter := &bloomfilter.Filter{}
	require.NoError(bloomFilter.UnmarshalBinary(pullGossipRequest.Filter))
	gossipBloomFilter := &gossip.BloomFilter{
		Bloom: bloomFilter,
		Salt:  ids.ID(pullGossipRequest.Salt),
	}
	require.True(gossipBloomFilter.Has(tx1))
}

func TestNetworkServesInboundPullGossipRequest(t *testing.T) {
	require := require.New(t)
	ctrl := gomock.NewController(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	snowCtx := snow.DefaultContextTest()

	// only validators can request gossip
	nodeID := ids.GenerateTestNodeID()
	snowCtx.ValidatorState = &validators.TestState{
		GetCurrentHeightF: func(context.Context) (uint64, error) {
			return 0, nil
		},
		GetValidatorSetF: func(context.Context, uint64, ids.ID) (map[ids.NodeID]*validators.GetValidatorOutput, error) {
			return map[ids.NodeID]*validators.GetValidatorOutput{
				nodeID: nil,
			}, nil
		},
	}

	mockVerifier := executor.NewMockManager(ctrl)
	mockVerifier.EXPECT().VerifyTx(gomock.Any()).Return(nil).Times(2)

	metrics := prometheus.NewRegistry()
	mempool, err := mempool.New("", metrics, nil)
	require.NoError(err)

	sender := &common.FakeSender{
		SentAppResponse: make(chan []byte, 1),
	}
	network, err := New(
		snowCtx,
		mockVerifier,
		mempool,
		false,
		sender,
		txGossipThrottlingPeriod,
		txGossipThrottlingLimit,
		txGossipBloomMaxItems,
		txGossipFalsePositiveRate,
		txGossipMaxFalsePositiveRate,
		metrics,
	)
	require.NoError(network.Connected(ctx, nodeID, nil))

	require.NoError(err)

	tx1, err := testTx(1)
	require.NoError(err)

	tx2, err := testTx(2)
	require.NoError(err)

	// I know about tx1 and tx2
	require.NoError(network.mempool.Add(tx1))
	require.NoError(network.mempool.Add(tx2))

	// the requester only knows about tx1
	bloom, err := gossip.NewBloomFilter(txGossipBloomMaxItems, txGossipFalsePositiveRate)
	require.NoError(err)
	bloom.Add(tx1)

	bloomBytes, err := bloom.Bloom.MarshalBinary()
	require.NoError(err)
	pullGossipRequest := &sdk.PullGossipRequest{
		Filter: bloomBytes,
		Salt:   bloom.Salt[:],
	}
	pullGossipRequestBytes, err := proto.Marshal(pullGossipRequest)
	require.NoError(err)

	inboundMsgBytes := append(binary.AppendUvarint(nil, txGossipHandlerID), pullGossipRequestBytes...)
	require.NoError(network.AppRequest(ctx, nodeID, 1, time.Time{}, inboundMsgBytes))

	pullGossipResponseBytes := <-sender.SentAppResponse
	pullGossipResponse := &sdk.PullGossipResponse{}
	require.NoError(proto.Unmarshal(pullGossipResponseBytes, pullGossipResponse))
	require.Len(pullGossipResponse.Gossip, 1)

	gotTx := &txs.Tx{}
	require.NoError(gotTx.Unmarshal(pullGossipResponse.Gossip[0]))
	require.Equal(tx2.ID(), gotTx.ID())
}

func TestNetworkHandlesPullGossipResponse(t *testing.T) {
	require := require.New(t)
	ctrl := gomock.NewController(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	snowCtx := snow.DefaultContextTest()
	snowCtx.ValidatorState = &validators.TestState{
		GetCurrentHeightF: func(context.Context) (uint64, error) {
			return 0, nil
		},
		GetValidatorSetF: func(context.Context, uint64, ids.ID) (map[ids.NodeID]*validators.GetValidatorOutput, error) {
			return map[ids.NodeID]*validators.GetValidatorOutput{
				snowCtx.NodeID: nil,
			}, nil
		},
	}

	mockVerifier := executor.NewMockManager(ctrl)
	metrics := prometheus.NewRegistry()
	mempool, err := mempool.New("", metrics, nil)
	require.NoError(err)

	sender := &common.FakeSender{
		SentAppRequest: make(chan []byte, txGossipPollSize),
	}
	network, err := New(
		snowCtx,
		mockVerifier,
		mempool,
		false,
		sender,
		txGossipThrottlingPeriod,
		txGossipThrottlingLimit,
		txGossipBloomMaxItems,
		txGossipFalsePositiveRate,
		txGossipMaxFalsePositiveRate,
		metrics,
	)
	require.NoError(err)
	require.NoError(network.Connected(ctx, snowCtx.NodeID, nil))

	go network.Gossip(ctx)

	tx1, err := testTx(1)
	require.NoError(err)

	<-sender.SentAppRequest

	pullGossipResponse := &sdk.PullGossipResponse{
		Gossip: [][]byte{tx1.Bytes()},
	}
	pullGossipResponseBytes, err := proto.Marshal(pullGossipResponse)
	require.NoError(err)

	inboundResponseBytes := append(txGossipHandlerPrefix, pullGossipResponseBytes...)
	require.NoError(network.AppResponse(ctx, snowCtx.NodeID, 1, inboundResponseBytes))
	network.mempool.Mempool.Has(tx1.ID())
}

func testTx(i int) (*txs.Tx, error) {
	pk, err := secp256k1.NewPrivateKey()
	if err != nil {
		return nil, err
	}

	utx := &txs.CreateChainTx{
		BaseTx: txs.BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    10,
			BlockchainID: ids.Empty.Prefix(uint64(i)),
			Ins: []*avax.TransferableInput{{
				UTXOID: avax.UTXOID{
					TxID:        ids.ID{'t', 'x', 'I', 'D'},
					OutputIndex: uint32(i),
				},
				Asset: avax.Asset{ID: ids.ID{'a', 's', 's', 'e', 'r', 't'}},
				In: &secp256k1fx.TransferInput{
					Amt:   uint64(5678),
					Input: secp256k1fx.Input{SigIndices: []uint32{uint32(i)}},
				},
			}},
			Outs: []*avax.TransferableOutput{{
				Asset: avax.Asset{ID: ids.ID{'a', 's', 's', 'e', 'r', 't'}},
				Out: &secp256k1fx.TransferOutput{
					Amt: uint64(1234),
					OutputOwners: secp256k1fx.OutputOwners{
						Threshold: 1,
						Addrs:     []ids.ShortID{pk.Address()},
					},
				},
			}},
		}},
		SubnetID:    ids.GenerateTestID(),
		ChainName:   "chainName",
		VMID:        ids.GenerateTestID(),
		FxIDs:       []ids.ID{ids.GenerateTestID()},
		GenesisData: []byte{'g', 'e', 'n', 'D', 'a', 't', 'a'},
		SubnetAuth:  &secp256k1fx.Input{SigIndices: []uint32{1}},
	}

	return txs.NewSigned(utx, txs.Codec, [][]*secp256k1.PrivateKey{{pk}})
}
