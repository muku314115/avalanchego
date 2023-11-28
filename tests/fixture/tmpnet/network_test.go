// Copyright (C) 2019-2023, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package tmpnet

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNetworkSerialization(t *testing.T) {
	require := require.New(t)

	tmpDir := t.TempDir()

	network, err := NewDefaultNetwork(&bytes.Buffer{}, "/path/to/avalanche/go", 1)
	require.NoError(err)
	require.NoError(network.Create(tmpDir))

	// Ensure node runtime is initialized
	require.NoError(network.readNodes())

	loadedNetwork, err := ReadNetwork(network.Dir)
	require.NoError(err)
	for _, key := range loadedNetwork.PreFundedKeys {
		// Address() enables comparison with the original network by
		// ensuring full population of a key's in-memory representation.
		_ = key.Address()
	}
	require.Equal(network, loadedNetwork)
}
