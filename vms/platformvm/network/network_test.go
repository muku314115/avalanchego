// Copyright (C) 2019-2023, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package network

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/network/p2p"
	"github.com/ava-labs/avalanchego/network/p2p/gossip"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/snow/engine/common"
	snowvalidators "github.com/ava-labs/avalanchego/snow/validators"
	"github.com/ava-labs/avalanchego/utils/crypto/secp256k1"
	"github.com/ava-labs/avalanchego/utils/set"
	"github.com/ava-labs/avalanchego/vms/components/avax"
	"github.com/ava-labs/avalanchego/vms/components/message"
	"github.com/ava-labs/avalanchego/vms/platformvm/block/executor"
	"github.com/ava-labs/avalanchego/vms/platformvm/txs"
	"github.com/ava-labs/avalanchego/vms/platformvm/txs/mempool"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
)

const (
	maxValidatorSetStaleness     = time.Second
	txGossipHandlerID            = 0
	txGossipFrequency            = time.Second
	txGossipThrottlingPeriod     = time.Second
	txGossipThrottlingLimit      = 1
	txGossipBloomMaxItems        = 10
	txGossipFalsePositiveRate    = 0.1
	txGossipMaxFalsePositiveRate = 0.5
)

var (
	errTest = errors.New("test error")

	_ Mempool = (*testMempool)(nil)
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
		managerFunc               func(*gomock.Controller) executor.Manager
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
			managerFunc: func(*gomock.Controller) executor.Manager {
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
			managerFunc: func(*gomock.Controller) executor.Manager {
				return nil
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
				mempool.EXPECT().Add(gomock.Any()).Return(nil)
				mempool.EXPECT().GetDropReason(gomock.Any()).Return(nil)
				mempool.EXPECT().RequestBuildBlock(gomock.Any())
				return mempool
			},
			managerFunc: func(ctrl *gomock.Controller) executor.Manager {
				manager := executor.NewMockManager(ctrl)
				manager.EXPECT().VerifyTx(gomock.Any()).Return(nil)
				return manager
			},
			appSenderFunc: func(ctrl *gomock.Controller) common.AppSender {
				appSender := common.NewMockSender(ctrl)
				appSender.EXPECT().SendAppGossip(gomock.Any(), gomock.Any()).Times(1)
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
			managerFunc: func(*gomock.Controller) executor.Manager {
				return nil
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
			managerFunc: func(*gomock.Controller) executor.Manager {
				return nil
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

			mempool, err := NewVerifierMempool(
				tt.mempoolFunc(ctrl),
				tt.managerFunc(ctrl),
				txGossipBloomMaxItems,
				txGossipFalsePositiveRate,
				txGossipMaxFalsePositiveRate,
			)
			require.NoError(err)

			snowCtx := snow.DefaultContextTest()
			sender := tt.appSenderFunc(ctrl)
			p2pNetwork, err := p2p.NewNetwork(
				snowCtx.Log,
				sender,
				prometheus.NewRegistry(),
				"",
			)
			require.NoError(err)

			validators := p2p.NewValidators(
				p2pNetwork.Peers,
				snowCtx.Log,
				snowCtx.SubnetID,
				snowCtx.ValidatorState,
				maxValidatorSetStaleness,
			)

			n, err := New(
				snowCtx,
				mempool,
				tt.partialSyncPrimaryNetwork,
				sender,
				p2pNetwork,
				validators,
				gossip.NoOpAccumulator[*txs.Tx]{},
				gossip.NoOpGossiper{},
				p2p.NoOpHandler{},
				txGossipHandlerID,
				txGossipFrequency,
				txGossipThrottlingPeriod,
				txGossipThrottlingLimit,
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
				return common.NewMockSender(ctrl)
			},
			expectedErr: errTest,
		},
		{
			name: "transaction invalid",
			mempoolFunc: func(ctrl *gomock.Controller) mempool.Mempool {
				mempool := mempool.NewMockMempool(ctrl)
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
				appSender.EXPECT().SendAppGossip(gomock.Any(), gomock.Any()).Times(1)
				return appSender
			},
			expectedErr: nil,
		},
		{
			name: "happy path",
			mempoolFunc: func(ctrl *gomock.Controller) mempool.Mempool {
				mempool := mempool.NewMockMempool(ctrl)
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
				appSender.EXPECT().SendAppGossip(gomock.Any(), gomock.Any()).Times(1)
				return appSender
			},
			expectedErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)
			ctrl := gomock.NewController(t)

			mempool, err := NewVerifierMempool(
				tt.mempoolFunc(ctrl),
				tt.managerFunc(ctrl),
				txGossipBloomMaxItems,
				txGossipFalsePositiveRate,
				txGossipMaxFalsePositiveRate,
			)
			require.NoError(err)

			snowCtx := snow.DefaultContextTest()
			sender := tt.appSenderFunc(ctrl)
			p2pNetwork, err := p2p.NewNetwork(
				snowCtx.Log,
				sender,
				prometheus.NewRegistry(),
				"",
			)
			require.NoError(err)

			validators := p2p.NewValidators(
				p2pNetwork.Peers,
				snowCtx.Log,
				snowCtx.SubnetID,
				snowCtx.ValidatorState,
				maxValidatorSetStaleness,
			)

			n, err := New(
				snowCtx,
				mempool,
				tt.partialSyncPrimaryNetwork,
				sender,
				p2pNetwork,
				validators,
				gossip.NoOpAccumulator[*txs.Tx]{},
				gossip.NoOpGossiper{},
				p2p.NoOpHandler{},
				txGossipHandlerID,
				txGossipFrequency,
				txGossipThrottlingPeriod,
				txGossipThrottlingLimit,
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

	mempool, err := NewVerifierMempool(
		mempool.NewMockMempool(ctrl),
		executor.NewMockManager(ctrl),
		txGossipBloomMaxItems,
		txGossipFalsePositiveRate,
		txGossipMaxFalsePositiveRate,
	)
	require.NoError(err)

	snowCtx := snow.DefaultContextTest()
	appSender := common.NewMockSender(ctrl)
	p2pNetwork, err := p2p.NewNetwork(
		snowCtx.Log,
		appSender,
		prometheus.NewRegistry(),
		"",
	)
	require.NoError(err)

	validators := p2p.NewValidators(
		p2pNetwork.Peers,
		snowCtx.Log,
		snowCtx.SubnetID,
		snowCtx.ValidatorState,
		maxValidatorSetStaleness,
	)
	n, err := New(
		snowCtx,
		mempool,
		false,
		appSender,
		p2pNetwork,
		validators,
		gossip.NoOpAccumulator[*txs.Tx]{},
		gossip.NoOpGossiper{},
		p2p.NoOpHandler{},
		txGossipHandlerID,
		txGossipFrequency,
		txGossipThrottlingPeriod,
		txGossipThrottlingLimit,
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

// Tests that push gossip is used when a tx is issued
func TestNetworkOutboundPushGossip(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	mempool, err := newTestMempool()
	require.NoError(err)

	snowCtx := snow.DefaultContextTest()
	sender := &common.SenderTest{}
	p2pNetwork, err := p2p.NewNetwork(
		snowCtx.Log,
		sender,
		prometheus.NewRegistry(),
		"",
	)
	require.NoError(err)

	validators := p2p.NewValidators(
		p2pNetwork.Peers,
		snowCtx.Log,
		snowCtx.SubnetID,
		snowCtx.ValidatorState,
		maxValidatorSetStaleness,
	)

	wantTx, err := testTx(0)
	require.NoError(err)

	var added, gossiped bool

	network, err := New(
		snowCtx,
		mempool,
		false,
		sender,
		p2pNetwork,
		validators,
		gossip.TestAccumulator[*txs.Tx]{
			GossipF: func(context.Context) error {
				gossiped = true
				return nil
			},
			AddF: func(txs ...*txs.Tx) {
				require.Len(txs, 1)
				require.Equal(wantTx, txs[0])
				added = true
			},
		},
		gossip.NoOpGossiper{},
		p2p.NoOpHandler{},
		txGossipHandlerID,
		txGossipFrequency,
		txGossipThrottlingPeriod,
		txGossipThrottlingLimit,
	)
	require.NoError(err)

	require.NoError(network.IssueTx(ctx, wantTx))
	require.True(added)
	require.True(gossiped)
}

// Tests that incoming push gossip messages are routed to the push gossip
// handler
func TestNetworkInboundTxPushGossip(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	mempool, err := newTestMempool()
	require.NoError(err)

	snowCtx := snow.DefaultContextTest()
	sender := &common.SenderTest{}
	p2pNetwork, err := p2p.NewNetwork(
		snowCtx.Log,
		sender,
		prometheus.NewRegistry(),
		"",
	)
	require.NoError(err)

	validators := p2p.NewValidators(
		p2pNetwork.Peers,
		snowCtx.Log,
		snowCtx.SubnetID,
		snowCtx.ValidatorState,
		maxValidatorSetStaleness,
	)

	wantNodeID := ids.GenerateTestNodeID()
	wantGossipBytes := []byte{1, 2, 3}
	inboundGossipMsg := append([]byte{byte(txGossipHandlerID)}, wantGossipBytes...)

	var called bool
	network, err := New(
		snowCtx,
		mempool,
		false,
		sender,
		p2pNetwork,
		validators,
		gossip.NoOpAccumulator[*txs.Tx]{},
		gossip.NoOpGossiper{},
		p2p.TestHandler{
			AppGossipF: func(ctx context.Context, nodeID ids.NodeID, gossipBytes []byte) {
				require.Equal(wantNodeID, nodeID)
				require.Equal(wantGossipBytes, gossipBytes)
				called = true
			},
		},
		txGossipHandlerID,
		txGossipFrequency,
		txGossipThrottlingPeriod,
		txGossipThrottlingLimit,
	)
	require.NoError(err)

	require.NoError(network.AppGossip(ctx, wantNodeID, inboundGossipMsg))
	require.True(called)
}

// Tests that an inbound pull gossip request is routed to the correct handler
func TestNetworkServesInboundPullGossipRequest(t *testing.T) {
	tests := []struct {
		name       string
		validators []ids.NodeID
		nodeID     ids.NodeID
		expected   bool
	}{
		{
			name:       "dropped - not a validator",
			validators: []ids.NodeID{},
			nodeID:     ids.EmptyNodeID,
		},
		{
			name:       "served - validator",
			validators: []ids.NodeID{ids.EmptyNodeID},
			nodeID:     ids.EmptyNodeID,
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)
			ctx := context.Background()

			mempool, err := newTestMempool()
			require.NoError(err)

			validatorSet := make(map[ids.NodeID]*snowvalidators.GetValidatorOutput)
			for _, validator := range tt.validators {
				validatorSet[validator] = nil
			}
			snowCtx := snow.DefaultContextTest()
			snowCtx.ValidatorState = &snowvalidators.TestState{
				GetCurrentHeightF: func(context.Context) (uint64, error) {
					return 0, nil
				},
				GetValidatorSetF: func(context.Context, uint64, ids.ID) (map[ids.NodeID]*snowvalidators.GetValidatorOutput, error) {
					return validatorSet, nil
				},
			}
			sender := &common.SenderTest{
				SendAppResponseF: func(context.Context, ids.NodeID, uint32, []byte) error {
					return nil
				},
			}
			p2pNetwork, err := p2p.NewNetwork(
				snowCtx.Log,
				sender,
				prometheus.NewRegistry(),
				"",
			)
			require.NoError(err)

			validators := p2p.NewValidators(
				p2pNetwork.Peers,
				snowCtx.Log,
				snowCtx.SubnetID,
				snowCtx.ValidatorState,
				maxValidatorSetStaleness,
			)

			wantDeadline := time.Now()
			wantRequestBytes := []byte{1, 2, 3}
			inboundRequest := append([]byte{byte(txGossipHandlerID)}, wantRequestBytes...)

			var called bool
			network, err := New(
				snowCtx,
				mempool,
				false,
				sender,
				p2pNetwork,
				validators,
				gossip.NoOpAccumulator[*txs.Tx]{},
				gossip.NoOpGossiper{},
				p2p.TestHandler{
					AppRequestF: func(ctx context.Context, nodeID ids.NodeID, deadline time.Time, requestBytes []byte) ([]byte, error) {
						require.Equal(tt.nodeID, nodeID)
						require.Equal(wantDeadline, deadline)
						require.Equal(wantRequestBytes, requestBytes)
						called = true
						return nil, nil
					},
				},
				txGossipHandlerID,
				txGossipFrequency,
				txGossipThrottlingPeriod,
				txGossipThrottlingLimit,
			)
			require.NoError(err)
			require.NoError(network.Connected(ctx, tt.nodeID, nil))

			require.NoError(network.AppRequest(ctx, tt.nodeID, 0, wantDeadline, inboundRequest))
			require.Equal(tt.expected, called)
		})
	}
}

func TestNetworkOutboundTxPullGossip(t *testing.T) {
	tests := []struct {
		name       string
		validators []ids.NodeID
		nodeID     ids.NodeID
		expected   bool
	}{
		{
			name:       "does not gossip - not a validator",
			validators: []ids.NodeID{},
			nodeID:     ids.EmptyNodeID,
		},
		{
			name:       "gossips - validator",
			validators: []ids.NodeID{ids.EmptyNodeID},
			nodeID:     ids.EmptyNodeID,
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			mempool, err := newTestMempool()
			require.NoError(err)

			validatorSet := make(map[ids.NodeID]*snowvalidators.GetValidatorOutput)
			for _, validator := range tt.validators {
				validatorSet[validator] = nil
			}
			snowCtx := snow.DefaultContextTest()
			snowCtx.ValidatorState = &snowvalidators.TestState{
				GetCurrentHeightF: func(context.Context) (uint64, error) {
					return 0, nil
				},
				GetValidatorSetF: func(context.Context, uint64, ids.ID) (map[ids.NodeID]*snowvalidators.GetValidatorOutput, error) {
					return validatorSet, nil
				},
			}
			sender := &common.SenderTest{}
			p2pNetwork, err := p2p.NewNetwork(
				snowCtx.Log,
				sender,
				prometheus.NewRegistry(),
				"",
			)
			require.NoError(err)
			require.NoError(p2pNetwork.Connected(ctx, tt.nodeID, nil))

			validators := p2p.NewValidators(
				p2pNetwork.Peers,
				snowCtx.Log,
				snowCtx.SubnetID,
				snowCtx.ValidatorState,
				maxValidatorSetStaleness,
			)

			var called bool
			network, err := New(
				snowCtx,
				mempool,
				false,
				sender,
				p2pNetwork,
				validators,
				gossip.NoOpAccumulator[*txs.Tx]{},
				&gossip.TestGossiper{
					GossipF: func(context.Context) error {
						called = true
						return nil
					},
				},
				p2p.NoOpHandler{},
				txGossipHandlerID,
				txGossipFrequency,
				txGossipThrottlingPeriod,
				txGossipThrottlingLimit,
			)
			require.NoError(err)
			require.NoError(network.Connected(ctx, tt.nodeID, nil))

			go network.Gossip(ctx)
			require.Eventually(func() bool { return tt.expected == called }, time.Minute, time.Second)
		})
	}
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

func newTestMempool() (*testMempool, error) {
	bloom, err := gossip.NewBloomFilter(txGossipBloomMaxItems, txGossipFalsePositiveRate)
	return &testMempool{
		dropped:  make(map[ids.ID]error),
		bloom:    bloom,
		building: false,
	}, err
}

type testMempool struct {
	mempool  set.Set[*txs.Tx]
	dropped  map[ids.ID]error
	bloom    *gossip.BloomFilter
	building bool
}

func (t *testMempool) Add(tx *txs.Tx) error {
	if t.mempool.Contains(tx) {
		return fmt.Errorf("tx %s already in mempool", tx.ID())
	}

	t.mempool.Add(tx)
	return nil
}

func (t *testMempool) Iterate(f func(tx *txs.Tx) bool) {
	for tx := range t.mempool {
		if !f(tx) {
			return
		}
	}
}

func (t *testMempool) GetFilter() (bloom []byte, salt []byte, err error) {
	bytes, err := t.bloom.Bloom.MarshalBinary()
	return bytes, t.bloom.Salt[:], err
}

func (t *testMempool) GetDropReason(txID ids.ID) error {
	reason, ok := t.dropped[txID]
	if ok {
		return reason
	}

	return nil
}

func (t *testMempool) RequestBuildBlock(allowEmptyBlock bool) {
	if allowEmptyBlock || len(t.mempool) > 0 {
		t.building = true
	}
}
