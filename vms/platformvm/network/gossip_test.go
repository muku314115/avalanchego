// Copyright (C) 2019-2023, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package network

import (
	"errors"
	"testing"

	bloomfilter "github.com/holiman/bloomfilter/v2"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/network/p2p/gossip"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/vms/platformvm/block/executor"
	"github.com/ava-labs/avalanchego/vms/platformvm/txs"
	"github.com/ava-labs/avalanchego/vms/platformvm/txs/mempool"
)

var errFoo = errors.New("foo")

// Add should error if verification against tip errors
func TestVerifierMempoolVerificationError(t *testing.T) {
	require := require.New(t)
	ctrl := gomock.NewController(t)

	tx := &Tx{
		Tx: &txs.Tx{
			TxID: ids.GenerateTestID(),
		},
	}

	snowCtx := snow.DefaultContextTest()
	mempool := mempool.NewMockMempool(ctrl)
	verifier := executor.NewMockManager(ctrl)

	mempool.EXPECT().Has(tx.ID()).Return(false)
	verifier.EXPECT().VerifyTx(tx.Tx).Return(errFoo)
	mempool.EXPECT().MarkDropped(tx.ID(), errFoo)

	verifierMempool, err := NewVerifierMempool(
		mempool,
		snowCtx,
		verifier,
		txGossipBloomMaxItems,
		txGossipFalsePositiveRate,
		txGossipMaxFalsePositiveRate,
	)
	require.NoError(err)
	err = verifierMempool.Add(tx)
	require.ErrorIs(err, errFoo)
}

// Add should error if adding to the mempool errors
func TestVerifierMempoolAddError(t *testing.T) {
	require := require.New(t)
	ctrl := gomock.NewController(t)

	tx := &Tx{
		Tx: &txs.Tx{
			TxID: ids.GenerateTestID(),
		},
	}

	snowCtx := snow.DefaultContextTest()
	verifier := executor.NewMockManager(ctrl)
	mempool := mempool.NewMockMempool(ctrl)

	mempool.EXPECT().Has(tx.ID()).Return(false)
	verifier.EXPECT().VerifyTx(tx.Tx).Return(nil)
	mempool.EXPECT().Add(tx.Tx).Return(errFoo)
	mempool.EXPECT().MarkDropped(tx.ID(), errFoo).AnyTimes()

	verifierMempool, err := NewVerifierMempool(
		mempool,
		snowCtx,
		verifier,
		txGossipBloomMaxItems,
		txGossipFalsePositiveRate,
		txGossipMaxFalsePositiveRate,
	)
	require.NoError(err)
	err = verifierMempool.Add(tx)
	require.ErrorIs(err, errFoo)
}

// Adding a duplicate to the mempool should return an error
func TestVerifierMempoolHas(t *testing.T) {
	require := require.New(t)
	ctrl := gomock.NewController(t)

	snowCtx := snow.DefaultContextTest()
	mempool := mempool.NewMockMempool(ctrl)
	verifier := executor.NewMockManager(ctrl)

	tx := &Tx{
		Tx: &txs.Tx{
			TxID: ids.GenerateTestID(),
		},
	}

	mempool.EXPECT().Has(tx.ID()).Return(true)
	mempool.EXPECT().MarkDropped(tx.ID(), gomock.Any())

	verifierMempool, err := NewVerifierMempool(
		mempool,
		snowCtx,
		verifier,
		txGossipBloomMaxItems,
		txGossipFalsePositiveRate,
		txGossipMaxFalsePositiveRate,
	)
	require.NoError(err)

	err = verifierMempool.Add(tx)
	require.ErrorIs(err, ErrTxPending)
}

// Adding a tx to the mempool should add it to the bloom filter
func TestVerifierMempoolAddBloomFilter(t *testing.T) {
	require := require.New(t)
	ctrl := gomock.NewController(t)

	tx := &Tx{
		Tx: &txs.Tx{
			TxID: ids.GenerateTestID(),
		},
	}

	snowCtx := snow.DefaultContextTest()
	verifier := executor.NewMockManager(ctrl)
	mempool := mempool.NewMockMempool(ctrl)

	mempool.EXPECT().Has(tx.ID()).Return(false)
	verifier.EXPECT().VerifyTx(tx.Tx).Return(nil)
	mempool.EXPECT().Add(tx.Tx).Return(nil)

	verifierMempool, err := NewVerifierMempool(
		mempool,
		snowCtx,
		verifier,
		txGossipBloomMaxItems,
		txGossipFalsePositiveRate,
		txGossipMaxFalsePositiveRate,
	)
	require.NoError(err)

	require.NoError(verifierMempool.Add(tx))

	bloomBytes, salt, err := verifierMempool.GetFilter()
	require.NoError(err)

	bloom := &gossip.BloomFilter{
		Bloom: &bloomfilter.Filter{},
		Salt:  ids.ID(salt),
	}

	require.NoError(bloom.Bloom.UnmarshalBinary(bloomBytes))
	require.True(bloom.Has(tx))
}
