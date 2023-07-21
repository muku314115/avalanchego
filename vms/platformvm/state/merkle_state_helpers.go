// Copyright (C) 2019-2023, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package state

import (
	"time"

	"github.com/ava-labs/avalanchego/database"
	"github.com/ava-labs/avalanchego/ids"
)

// helpers types to store data on merkleDB
type uptimes struct {
	Duration    time.Duration `serialize:"true"`
	LastUpdated uint64        `serialize:"true"` // Unix time in seconds

	// txID        ids.ID // TODO ABENEGIA: is it needed by delegators and not validators?
	lastUpdated time.Time
}

type stakersData struct {
	TxBytes         []byte `serialize:"true"`
	PotentialReward uint64 `serialize:"true"`
}

type weightDiffKey struct {
	subnetID ids.ID
	nodeID   ids.NodeID
}

func merkleSuppliesKeyPrefix() []byte {
	prefix := make([]byte, 0, len(merkleSuppliesPrefix))
	copy(prefix, merkleSuppliesPrefix)
	return prefix
}

func merkleSuppliesKey(subnetID ids.ID) []byte {
	key := merkleSuppliesKeyPrefix()
	key = append(key, subnetID[:]...)
	return key
}

func splitMerkleSuppliesKey(b []byte) ([]byte, ids.ID) {
	prefix := b[:len(merkleSuppliesPrefix)]
	subnetID := ids.Empty
	copy(subnetID[:], b[len(merkleSuppliesPrefix):])
	return prefix, subnetID
}

func merklePermissionedSubnetKey(subnetID ids.ID) []byte {
	key := make([]byte, 0, len(permissionedSubnetSectionPrefix)+len(subnetID[:]))
	copy(key, permissionedSubnetSectionPrefix)
	key = append(key, subnetID[:]...)
	return key
}

func merkleElasticSubnetKey(subnetID ids.ID) []byte {
	key := make([]byte, 0, len(elasticSubnetSectionPrefix)+len(subnetID[:]))
	copy(key, elasticSubnetSectionPrefix)
	key = append(key, subnetID[:]...)
	return key
}

func merkleChainPrefix(subnetID ids.ID) []byte {
	prefix := make([]byte, 0, len(chainsSectionPrefix)+len(subnetID[:]))
	copy(prefix, chainsSectionPrefix)
	prefix = append(prefix, subnetID[:]...)
	return prefix
}

func merkleChainKey(subnetID ids.ID, chainID ids.ID) []byte {
	prefix := merkleChainPrefix(subnetID)

	key := make([]byte, 0, len(prefix)+len(chainID))
	copy(key, prefix)
	key = append(key, chainID[:]...)
	return key
}

func merkleUtxoIDKey(utxoID ids.ID) []byte {
	key := make([]byte, 0, len(utxosSectionPrefix)+len(utxoID))
	copy(key, utxosSectionPrefix)
	key = append(key, utxoID[:]...)
	return key
}

func merkleRewardUtxosIDPrefix(txID ids.ID) []byte {
	prefix := make([]byte, 0, len(rewardUtxosSectionPrefix)+len(txID))
	copy(prefix, rewardUtxosSectionPrefix)
	prefix = append(prefix, txID[:]...)
	return prefix
}

func merkleRewardUtxoIDKey(txID, utxoID ids.ID) []byte {
	prefix := merkleRewardUtxosIDPrefix(txID)
	key := make([]byte, 0, len(prefix)+len(utxoID))
	copy(key, prefix)
	key = append(key, utxoID[:]...)
	return key
}

func merkleUtxoIndexKey(address []byte, utxoID ids.ID) []byte {
	key := make([]byte, 0, len(address)+len(utxoID))
	copy(key, address)
	key = append(key, utxoID[:]...)
	return key
}

func merkleLocalUptimesKey(nodeID ids.NodeID, subnetID ids.ID) []byte {
	key := make([]byte, 0, len(nodeID)+len(subnetID))
	copy(key, nodeID[:])
	key = append(key, subnetID[:]...)
	return key
}

func merkleCurrentStakersKey(txID ids.ID) []byte {
	key := make([]byte, 0, len(currentStakersSectionPrefix)+len(txID))
	copy(key, currentStakersSectionPrefix)
	key = append(key, txID[:]...)
	return key
}

func merklePendingStakersKey(txID ids.ID) []byte {
	key := make([]byte, 0, len(pendingStakersSectionPrefix)+len(txID))
	copy(key, pendingStakersSectionPrefix)
	key = append(key, txID[:]...)
	return key
}

func merkleDelegateeRewardsKey(nodeID ids.NodeID, subnetID ids.ID) []byte {
	key := make([]byte, 0, len(delegateeRewardsPrefix)+len(nodeID)+len(subnetID))
	copy(key, delegateeRewardsPrefix)
	key = append(key, nodeID[:]...)
	key = append(key, subnetID[:]...)
	return key
}

func merkleWeightDiffKey(subnetID ids.ID, nodeID ids.NodeID, height uint64) []byte {
	key := make([]byte, 0, len(nodeID)+len(subnetID)) // missing height part
	key = append(key, subnetID[:]...)
	key = append(key, nodeID[:]...)
	key = append(key, database.PackUInt64(height)...)
	return key
}

// TODO: remove when ValidatorDiff optimization is merged in
func splitMerkleWeightDiffKey(key []byte) (ids.ID, ids.NodeID, uint64, error) {
	subnetIDLenght := 32
	nodeIDLenght := 20

	subnetID := ids.Empty
	copy(subnetID[:], key[0:subnetIDLenght])

	nodeID := ids.EmptyNodeID
	copy(nodeID[:], key[subnetIDLenght:subnetIDLenght+nodeIDLenght])

	height, err := database.ParseUInt64(key[subnetIDLenght+nodeIDLenght:])
	return subnetID, nodeID, height, err
}

func merkleBlsKeytDiffKey(nodeID ids.NodeID, height uint64) []byte {
	key := make([]byte, 0, len(nodeID)) // missing height part
	key = append(key, nodeID[:]...)
	key = append(key, database.PackUInt64(height)...)
	return key
}

// TODO: remove when ValidatorDiff optimization is merged in
func splitMerkleBlsKeyDiffKey(key []byte) (ids.NodeID, uint64, error) {
	nodeIDLenght := 20

	nodeID := ids.EmptyNodeID
	copy(nodeID[:], key[0:nodeIDLenght])

	height, err := database.ParseUInt64(key[nodeIDLenght:])
	return nodeID, height, err
}
