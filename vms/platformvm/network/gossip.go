// Copyright (C) 2019-2023, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package network

import (
	"context"
	"sync"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/network/p2p"
	"github.com/ava-labs/avalanchego/network/p2p/gossip"
	"github.com/ava-labs/avalanchego/utils/units"
	blockexecutor "github.com/ava-labs/avalanchego/vms/platformvm/block/executor"
	"github.com/ava-labs/avalanchego/vms/platformvm/txs"
	"github.com/ava-labs/avalanchego/vms/platformvm/txs/mempool"
)

const (
	maxValidatorSetStaleness = time.Minute
	txGossipHandlerID        = 0
	txGossipMaxGossipSize    = 20 * units.KiB
	txGossipPollSize         = 10
	txGossipFrequency        = 15 * time.Second
)

var (
	_ p2p.Handler         = (*txGossipHandler)(nil)
	_ gossip.Set[*txs.Tx] = (*verifierMempool)(nil)
)

// txGossipHandler is the handler called when serving gossip messages
type txGossipHandler struct {
	p2p.NoOpHandler
	appGossipHandler  p2p.Handler
	appRequestHandler p2p.Handler
}

func (t txGossipHandler) AppGossip(
	ctx context.Context,
	nodeID ids.NodeID,
	gossipBytes []byte,
) {
	t.appGossipHandler.AppGossip(ctx, nodeID, gossipBytes)
}

func (t txGossipHandler) AppRequest(
	ctx context.Context,
	nodeID ids.NodeID,
	deadline time.Time,
	requestBytes []byte,
) ([]byte, error) {
	return t.appRequestHandler.AppRequest(ctx, nodeID, deadline, requestBytes)
}

func newVerifierMempool(
	mempool mempool.Mempool,
	verifier blockexecutor.Manager,
	bloomMaxItems uint64,
	bloomFalsePositiveRate float64,
	bloomMaxFalsePositiveRate float64,
) (*verifierMempool, error) {
	bloomFilter, err := gossip.NewBloomFilter(
		bloomMaxItems,
		bloomFalsePositiveRate,
	)
	if err != nil {
		return nil, err
	}

	return &verifierMempool{
		Mempool:                   mempool,
		verifier:                  verifier,
		bloomFilter:               bloomFilter,
		bloomMaxFalsePositiveRate: bloomMaxFalsePositiveRate,
	}, nil
}

// verifierMempool performs verification before adding something to the mempool
type verifierMempool struct {
	mempool.Mempool
	verifier blockexecutor.Manager

	lock                      sync.RWMutex
	bloomFilter               *gossip.BloomFilter
	bloomMaxFalsePositiveRate float64
}

func (v *verifierMempool) Add(tx *txs.Tx) error {
	if err := v.verifier.VerifyTx(tx); err != nil {
		v.Mempool.MarkDropped(tx.ID(), err)
		return err
	}

	if err := v.Mempool.Add(tx); err != nil {
		v.Mempool.MarkDropped(tx.ID(), err)
		return err
	}

	v.lock.Lock()
	defer v.lock.Unlock()

	v.bloomFilter.Add(tx)

	ok, err := gossip.ResetBloomFilterIfNeeded(v.bloomFilter, v.bloomMaxFalsePositiveRate)
	if ok {
		v.Iterate(func(tx *txs.Tx) bool {
			v.bloomFilter.Add(tx)
			return true
		})
	}

	return err
}

func (v *verifierMempool) GetFilter() (bloom []byte, salt []byte, err error) {
	v.lock.RLock()
	defer v.lock.RUnlock()

	bloomBytes, err := v.bloomFilter.Bloom.MarshalBinary()
	return bloomBytes, v.bloomFilter.Salt[:], err
}
