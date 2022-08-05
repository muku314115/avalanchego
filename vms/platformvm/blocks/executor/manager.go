// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package executor

import (
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/consensus/snowman"
	"github.com/ava-labs/avalanchego/utils/window"
	"github.com/ava-labs/avalanchego/vms/platformvm/blocks"
	"github.com/ava-labs/avalanchego/vms/platformvm/metrics"
	"github.com/ava-labs/avalanchego/vms/platformvm/state"
	"github.com/ava-labs/avalanchego/vms/platformvm/txs/executor"
	"github.com/ava-labs/avalanchego/vms/platformvm/txs/mempool"
)

var _ Manager = &manager{}

type Manager interface {
	state.Versions

	// Returns the ID of the most recently accepted block.
	LastAccepted() ids.ID
	GetBlock(blkID ids.ID) (snowman.Block, error)
	NewBlock(blocks.Block) snowman.Block
}

func NewManager(
	mempool mempool.Mempool,
	metrics metrics.Metrics,
	s state.State,
	txExecutorBackend executor.Backend,
	recentlyAccepted *window.Window,
) Manager {
	backend := &backend{
		Mempool:      mempool,
		lastAccepted: s.GetLastAccepted(),
		state:        s,
		bootstrapped: txExecutorBackend.Bootstrapped,
		ctx:          txExecutorBackend.Ctx,
		blkIDToState: map[ids.ID]*blockState{},
	}

	manager := &manager{
		backend: backend,
		verifier: &verifier{
			backend:           backend,
			txExecutorBackend: txExecutorBackend,
		},
		acceptor: &acceptor{
			backend:          backend,
			metrics:          metrics,
			recentlyAccepted: recentlyAccepted,
		},
		rejector: &rejector{backend: backend},
	}
	return manager
}

type manager struct {
	*backend
	verifier blocks.Visitor
	acceptor blocks.Visitor
	rejector blocks.Visitor
}

func (m *manager) GetBlock(blkID ids.ID) (snowman.Block, error) {
	// See if the block is in memory.
	if blk, ok := m.blkIDToState[blkID]; ok {
		return m.NewBlock(blk.statelessBlock), nil
	}
	// The block isn't in memory. Check the database.
	statelessBlk, _, err := m.backend.state.GetStatelessBlock(blkID)
	if err != nil {
		return nil, err
	}
	return m.NewBlock(statelessBlk), nil
}

func (m *manager) NewBlock(blk blocks.Block) snowman.Block {
	return &Block{
		manager: m,
		Block:   blk,
	}
}
