// Copyright (C) 2019-2023, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package executor

import (
	"time"

	"github.com/ava-labs/avalanchego/database"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/math"
	"github.com/ava-labs/avalanchego/vms/platformvm/state"
	"github.com/ava-labs/avalanchego/vms/platformvm/txs"
)

// GetNextStakerChangeTime returns the next time a staker will be either added
// or removed to/from the current validator set.
func GetNextStakerChangeTime(state state.Chain) (time.Time, error) {
	currentStakerIterator, err := state.GetCurrentStakerIterator()
	if err != nil {
		return time.Time{}, err
	}
	defer currentStakerIterator.Release()

	pendingStakerIterator, err := state.GetPendingStakerIterator()
	if err != nil {
		return time.Time{}, err
	}
	defer pendingStakerIterator.Release()

	hasCurrentStaker := currentStakerIterator.Next()
	hasPendingStaker := pendingStakerIterator.Next()
	switch {
	case hasCurrentStaker && hasPendingStaker:
		nextCurrentTime := currentStakerIterator.Value().NextTime
		nextPendingTime := pendingStakerIterator.Value().NextTime
		if nextCurrentTime.Before(nextPendingTime) {
			return nextCurrentTime, nil
		}
		return nextPendingTime, nil
	case hasCurrentStaker:
		return currentStakerIterator.Value().NextTime, nil
	case hasPendingStaker:
		return pendingStakerIterator.Value().NextTime, nil
	default:
		return time.Time{}, database.ErrNotFound
	}
}

// GetValidator returns information about the given validator, which may be a
// current validator or pending validator.
func GetValidator(state state.Chain, subnetID ids.ID, nodeID ids.NodeID) (*state.Staker, error) {
	validator, err := state.GetCurrentValidator(subnetID, nodeID)
	if err == nil {
		// This node is currently validating the subnet.
		return validator, nil
	}
	if err != database.ErrNotFound {
		// Unexpected error occurred.
		return nil, err
	}
	return state.GetPendingValidator(subnetID, nodeID)
}

// canDelegate returns true if [delegator] can be added as a delegator of
// [validator].
//
// A [delegator] can be added if:
// - [delegator]'s start time is not before [validator]'s start time
// - [delegator]'s end time is not after [validator]'s end time
// - the maximum total weight on [validator] will not exceed [weightLimit]
func canDelegate(
	state state.Chain,
	validator *state.Staker,
	weightLimit uint64,
	delegator *state.Staker,
) (bool, error) {
	if !txs.BoundedBy(
		delegator.StartTime,
		delegator.EndTime,
		validator.StartTime,
		validator.EndTime,
	) {
		return false, nil
	}
	if delegator.StakingPeriod > validator.StakingPeriod {
		return false, nil
	}

	maxWeight, err := GetMaxWeight(state, validator, delegator.StartTime, delegator.EndTime)
	if err != nil {
		return false, err
	}
	newMaxWeight, err := math.Add64(maxWeight, delegator.Weight)
	if err != nil {
		return false, err
	}
	return newMaxWeight <= weightLimit, nil
}

// GetMaxWeight returns the maximum total weight of the [validator], including
// its own weight, between [startTime] and [endTime].
// The weight changes are applied in the order they will be applied as chain
// time advances.
// Invariant:
// - [validator.StartTime] <= [startTime] < [endTime] <= [validator.EndTime]
func GetMaxWeight(
	chainState state.Chain,
	validator *state.Staker,
	startTime time.Time,
	endTime time.Time,
) (uint64, error) {
	currentDelegatorIterator, err := chainState.GetCurrentDelegatorIterator(validator.SubnetID, validator.NodeID)
	if err != nil {
		return 0, err
	}

	// TODO: We can optimize this by moving the current total weight to be
	//       stored in the validator state.
	//
	// Calculate the current total weight on this validator, including the
	// weight of the actual validator and the sum of the weights of all of the
	// currently active delegators.
	currentWeight := validator.Weight
	for currentDelegatorIterator.Next() {
		currentDelegator := currentDelegatorIterator.Value()

		currentWeight, err = math.Add64(currentWeight, currentDelegator.Weight)
		if err != nil {
			currentDelegatorIterator.Release()
			return 0, err
		}
	}
	currentDelegatorIterator.Release()

	currentDelegatorIterator, err = chainState.GetCurrentDelegatorIterator(validator.SubnetID, validator.NodeID)
	if err != nil {
		return 0, err
	}
	pendingDelegatorIterator, err := chainState.GetPendingDelegatorIterator(validator.SubnetID, validator.NodeID)
	if err != nil {
		currentDelegatorIterator.Release()
		return 0, err
	}
	delegatorChangesIterator := state.NewStakerDiffIterator(currentDelegatorIterator, pendingDelegatorIterator)
	defer delegatorChangesIterator.Release()

	// Iterate over the future stake weight changes and calculate the maximum
	// total weight on the validator, only including the points in the time
	// range [startTime, endTime].
	var currentMax uint64
	for delegatorChangesIterator.Next() {
		delegator, isAdded := delegatorChangesIterator.Value()
		// [delegator.NextTime] > [endTime]
		if delegator.NextTime.After(endTime) {
			// This delegation change (and all following changes) occurs after
			// [endTime]. Since we're calculating the max amount staked in
			// [startTime, endTime], we can stop.
			break
		}

		// [delegator.NextTime] >= [startTime]
		if !delegator.NextTime.Before(startTime) {
			// We have advanced time to be at the inside of the delegation
			// window. Make sure that the max weight is updated accordingly.
			currentMax = math.Max(currentMax, currentWeight)
		}

		var op func(uint64, uint64) (uint64, error)
		if isAdded {
			op = math.Add64
		} else {
			op = math.Sub[uint64]
		}
		currentWeight, err = op(currentWeight, delegator.Weight)
		if err != nil {
			return 0, err
		}
	}
	// Because we assume [startTime] < [endTime], we have advanced time to
	// be at the end of the delegation window. Make sure that the max weight is
	// updated accordingly.
	return math.Max(currentMax, currentWeight), nil
}