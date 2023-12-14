// Copyright (C) 2019-2023, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package network

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"

	"github.com/ava-labs/avalanchego/cache"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/network/p2p"
	"github.com/ava-labs/avalanchego/network/p2p/gossip"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/snow/engine/common"
	"github.com/ava-labs/avalanchego/vms/components/message"
	blockexecutor "github.com/ava-labs/avalanchego/vms/platformvm/block/executor"
	"github.com/ava-labs/avalanchego/vms/platformvm/txs"
	"github.com/ava-labs/avalanchego/vms/platformvm/txs/mempool"
)

// We allow [recentCacheSize] to be fairly large because we only store hashes
// in the cache, not entire transactions.
const recentCacheSize = 512

type Network struct {
	*p2p.Network

	ctx                       *snow.Context
	verifier                  blockexecutor.Manager
	mempool                   *verifierMempool
	partialSyncPrimaryNetwork bool
	appSender                 common.AppSender

	txPushGossiper gossip.Accumulator[*txs.Tx]
	txPullGossiper gossip.Gossiper

	// gossip related attributes
	recentTxsLock sync.Mutex
	recentTxs     *cache.LRU[ids.ID, struct{}]
}

// New returns an instance of Network. The caller supplies a context which
// can be cancelled to shut down the Network.
func New(
	snowCtx *snow.Context,
	verifier blockexecutor.Manager,
	mempool mempool.Mempool,
	partialSyncPrimaryNetwork bool,
	appSender common.AppSender,
	txGossipThrottlingPeriod time.Duration,
	txGossipThrottlingLimit int,
	bloomMaxItems uint64,
	bloomFalsePositiveRate float64,
	bloomMaxFalsePositiveRate float64,
	registerer prometheus.Registerer,
) (*Network, error) {
	p2pNetwork, err := p2p.NewNetwork(
		snowCtx.Log,
		appSender,
		registerer,
		"p2p",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize p2p network: %w", err)
	}

	validators := p2p.NewValidators(
		p2pNetwork.Peers,
		snowCtx.Log,
		snowCtx.SubnetID,
		snowCtx.ValidatorState,
		maxValidatorSetStaleness,
	)
	txGossipClient := p2pNetwork.NewClient(
		txGossipHandlerID,
		p2p.WithValidatorSampling(validators),
	)
	txGossipMetrics, err := gossip.NewMetrics(registerer, "tx")
	if err != nil {
		return nil, err
	}

	txPushGossiper := gossip.NewPushGossiper[*txs.Tx](
		txGossipClient,
		txGossipMetrics,
		txGossipMaxGossipSize,
	)

	verifierMempool, err := newVerifierMempool(
		mempool,
		verifier,
		bloomMaxItems,
		bloomFalsePositiveRate,
		bloomMaxFalsePositiveRate,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize verification mempool: %w", err)
	}

	var txPullGossiper gossip.Gossiper
	txPullGossiper = gossip.NewPullGossiper[txs.Tx, *txs.Tx](
		snowCtx.Log,
		verifierMempool,
		txGossipClient,
		txGossipMetrics,
		txGossipPollSize,
	)

	// Gossip requests are only served if a node is a validator
	txPullGossiper = gossip.ValidatorGossiper{
		Gossiper:   txPullGossiper,
		NodeID:     snowCtx.NodeID,
		Validators: validators,
	}

	handler := gossip.NewHandler[txs.Tx, *txs.Tx](
		snowCtx.Log,
		txPushGossiper,
		verifierMempool,
		txGossipMetrics,
		txGossipMaxGossipSize,
	)

	validatorHandler := p2p.NewValidatorHandler(
		p2p.NewThrottlerHandler(
			handler,
			p2p.NewSlidingWindowThrottler(
				txGossipThrottlingPeriod,
				txGossipThrottlingLimit,
			),
			snowCtx.Log,
		),
		validators,
		snowCtx.Log,
	)

	// We allow pushing txs between all peers, but only serve gossip requests
	// from validators
	txGossipHandler := txGossipHandler{
		appGossipHandler:  handler,
		appRequestHandler: validatorHandler,
	}

	if err := p2pNetwork.AddHandler(txGossipHandlerID, txGossipHandler); err != nil {
		return nil, err
	}

	return &Network{
		Network:                   p2pNetwork,
		ctx:                       snowCtx,
		verifier:                  verifier,
		mempool:                   verifierMempool,
		partialSyncPrimaryNetwork: partialSyncPrimaryNetwork,
		appSender:                 appSender,
		txPushGossiper:            txPushGossiper,
		txPullGossiper:            txPullGossiper,
		recentTxs:                 &cache.LRU[ids.ID, struct{}]{Size: recentCacheSize},
	}, nil
}

func (n *Network) Gossip(ctx context.Context) {
	wg := &sync.WaitGroup{}
	wg.Add(1)

	go func() {
		gossip.Every(ctx, n.ctx.Log, n.txPullGossiper, txGossipFrequency)
		wg.Done()
	}()

	<-ctx.Done()
	wg.Wait()
}

func (n *Network) AppGossip(ctx context.Context, nodeID ids.NodeID, msgBytes []byte) error {
	n.ctx.Log.Debug("called AppGossip message handler",
		zap.Stringer("nodeID", nodeID),
		zap.Int("messageLen", len(msgBytes)),
	)

	if n.partialSyncPrimaryNetwork {
		n.ctx.Log.Debug("dropping AppGossip message",
			zap.String("reason", "primary network is not being fully synced"),
		)
		return nil
	}

	msgIntf, err := message.Parse(msgBytes)
	if err != nil {
		n.ctx.Log.Debug("forwarding AppGossip to p2p network",
			zap.String("reason", "failed to parse message"),
		)

		return n.Network.AppGossip(ctx, nodeID, msgBytes)
	}

	msg, ok := msgIntf.(*message.Tx)
	if !ok {
		n.ctx.Log.Debug("dropping unexpected message",
			zap.Stringer("nodeID", nodeID),
		)
		return nil
	}

	tx, err := txs.Parse(txs.Codec, msg.Tx)
	if err != nil {
		n.ctx.Log.Verbo("received invalid tx",
			zap.Stringer("nodeID", nodeID),
			zap.Binary("tx", msg.Tx),
			zap.Error(err),
		)
		return nil
	}
	txID := tx.ID()

	// We need to grab the context lock here to avoid racy behavior with
	// transaction verification + mempool modifications.
	//
	// Invariant: tx should not be referenced again without the context lock
	// held to avoid any data races.
	n.ctx.Lock.Lock()
	defer n.ctx.Lock.Unlock()

	if reason := n.mempool.GetDropReason(txID); reason != nil {
		// If the tx is being dropped - just ignore it
		return nil
	}
	if err := n.issueTx(tx); err == nil {
		n.legacyGossipTx(ctx, txID, msgBytes)

		n.txPushGossiper.Add(tx)
		return n.txPushGossiper.Gossip(ctx)
	}
	return nil
}

// IssueTx verifies the transaction at the currently preferred state, adds
// it to the mempool, and gossips it to the network.
//
// Invariant: Assumes the context lock is held.
func (n *Network) IssueTx(ctx context.Context, tx *txs.Tx) error {
	if err := n.issueTx(tx); err != nil {
		return err
	}

	txBytes := tx.Bytes()
	msg := &message.Tx{
		Tx: txBytes,
	}
	msgBytes, err := message.Build(msg)
	if err != nil {
		return err
	}

	txID := tx.ID()
	n.legacyGossipTx(ctx, txID, msgBytes)
	n.txPushGossiper.Add(tx)
	return n.txPushGossiper.Gossip(ctx)
}

// returns nil if the tx is in the mempool
func (n *Network) issueTx(tx *txs.Tx) error {
	// If we are partially syncing the Primary Network, we should not be
	// maintaining the transaction mempool locally.
	if n.partialSyncPrimaryNetwork {
		return nil
	}

	txID := tx.ID()
	if n.mempool.Has(txID) {
		// The tx is already in the mempool
		return nil
	}

	if err := n.mempool.Add(tx); err != nil {
		n.ctx.Log.Debug("tx failed to be added to the mempool",
			zap.Stringer("txID", txID),
			zap.Error(err),
		)
		return err
	}

	n.mempool.RequestBuildBlock(false /*=emptyBlockPermitted*/)

	return nil
}

func (n *Network) legacyGossipTx(ctx context.Context, txID ids.ID, msgBytes []byte) {
	n.recentTxsLock.Lock()
	_, has := n.recentTxs.Get(txID)
	n.recentTxs.Put(txID, struct{}{})
	n.recentTxsLock.Unlock()

	// Don't gossip a transaction if it has been recently gossiped.
	if has {
		return
	}

	n.ctx.Log.Debug("gossiping tx",
		zap.Stringer("txID", txID),
	)

	if err := n.appSender.SendAppGossip(ctx, msgBytes); err != nil {
		n.ctx.Log.Error("failed to gossip tx",
			zap.Stringer("txID", txID),
			zap.Error(err),
		)
	}
}
