// Copyright (C) 2019-2023, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package network

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/ava-labs/avalanchego/cache"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/network/p2p"
	"github.com/ava-labs/avalanchego/network/p2p/gossip"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/snow/engine/common"
	"github.com/ava-labs/avalanchego/vms/components/message"
	"github.com/ava-labs/avalanchego/vms/platformvm/txs"
)

// We allow [recentCacheSize] to be fairly large because we only store hashes
// in the cache, not entire transactions.
const recentCacheSize = 512

type Mempool interface {
	gossip.Set[*Tx]
	GetDropReason(txID ids.ID) error
	RequestBuildBlock(emptyBlockPermitted bool)
}

type Network struct {
	*p2p.Network

	ctx                       *snow.Context
	mempool                   Mempool
	partialSyncPrimaryNetwork bool
	appSender                 common.AppSender

	txPushGossiper    gossip.Accumulator[*Tx]
	txPullGossiper    gossip.Gossiper
	txGossipFrequency time.Duration

	// gossip related attributes
	recentTxsLock sync.Mutex
	recentTxs     *cache.LRU[ids.ID, struct{}]
}

// New returns an instance of Network. The caller supplies a context which
// can be cancelled to shut down the Network.
func New(
	snowCtx *snow.Context,
	mempool Mempool,
	partialSyncPrimaryNetwork bool,
	appSender common.AppSender,
	p2pNetwork *p2p.Network,
	validators *p2p.Validators,
	txPushGossiper gossip.Accumulator[*Tx],
	txPullGossiper gossip.Gossiper,
	handler p2p.Handler,
	txGossipHandlerID uint64,
	txGossipFrequency time.Duration,
	txGossipThrottlingPeriod time.Duration,
	txGossipThrottlingLimit int,
) (*Network, error) {
	// Outbound Pull Gossip requests are only made if this node is a validator
	validatorTxPullGossiper := gossip.ValidatorGossiper{
		Gossiper:   txPullGossiper,
		NodeID:     snowCtx.NodeID,
		Validators: validators,
	}

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
		mempool:                   mempool,
		partialSyncPrimaryNetwork: partialSyncPrimaryNetwork,
		appSender:                 appSender,
		txPushGossiper:            txPushGossiper,
		txPullGossiper:            validatorTxPullGossiper,
		txGossipFrequency:         txGossipFrequency,
		recentTxs:                 &cache.LRU[ids.ID, struct{}]{Size: recentCacheSize},
	}, nil
}

func (n *Network) Gossip(ctx context.Context) {
	gossip.Every(ctx, n.ctx.Log, n.txPullGossiper, n.txGossipFrequency)
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
	if reason := n.mempool.GetDropReason(txID); reason != nil {
		// If the tx is being dropped - just ignore it
		return nil
	}

	gossipTx := &Tx{Tx: tx}
	if err := n.issueTx(gossipTx); err == nil {
		n.legacyGossipTx(ctx, txID, msgBytes)

		n.txPushGossiper.Add(gossipTx)
		return n.txPushGossiper.Gossip(ctx)
	}
	return nil
}

// IssueTx verifies the transaction at the currently preferred state, adds
// it to the mempool, and gossips it to the network.
//
// Invariant: Assumes the context lock is held.
func (n *Network) IssueTx(ctx context.Context, tx *txs.Tx) error {
	gossipTx := &Tx{Tx: tx}

	if err := n.issueTx(gossipTx); err != nil {
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
	n.txPushGossiper.Add(gossipTx)
	return n.txPushGossiper.Gossip(ctx)
}

// returns nil if the tx is in the mempool
func (n *Network) issueTx(tx *Tx) error {
	// If we are partially syncing the Primary Network, we should not be
	// maintaining the transaction mempool locally.
	if n.partialSyncPrimaryNetwork {
		return nil
	}

	txID := tx.ID()
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
