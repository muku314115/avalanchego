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
	verifier := executor.NewMockManager(ctrl)
	verifier.EXPECT().VerifyTx(tx).Return(errFoo)
	mempool := mempool.NewMockMempool(ctrl)
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
	verifier.EXPECT().VerifyTx(tx).Return(nil)
	mempool := mempool.NewMockMempool(ctrl)
	mempool.EXPECT().Add(tx).Return(errFoo)
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

// Adding a tx to the mempool should add it to the bloom filter
func TestVerifierMempoolAddBloomFilter(t *testing.T) {
	require := require.New(t)
	ctrl := gomock.NewController(t)

	snowCtx := snow.DefaultContextTest()
	verifier := executor.NewMockManager(ctrl)
	verifier.EXPECT().VerifyTx(gomock.Any()).Return(nil).AnyTimes()
	mempool := mempool.NewMockMempool(ctrl)
	mempool.EXPECT().Add(gomock.Any()).Return(nil).AnyTimes()

	verifierMempool, err := NewVerifierMempool(
		mempool,
		snowCtx,
		verifier,
		txGossipBloomMaxItems,
		txGossipFalsePositiveRate,
		txGossipMaxFalsePositiveRate,
	)
	require.NoError(err)

	tx := &Tx{
		Tx: &txs.Tx{
			TxID: ids.GenerateTestID(),
		},
	}
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
