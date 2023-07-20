// Copyright (C) 2019-2023, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package state

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ava-labs/avalanchego/cache"
	"github.com/ava-labs/avalanchego/cache/metercacher"
	"github.com/ava-labs/avalanchego/database"
	"github.com/ava-labs/avalanchego/database/prefixdb"
	"github.com/ava-labs/avalanchego/database/versiondb"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/validators"
	"github.com/ava-labs/avalanchego/trace"
	"github.com/ava-labs/avalanchego/utils/crypto/bls"
	"github.com/ava-labs/avalanchego/utils/set"
	"github.com/ava-labs/avalanchego/utils/wrappers"
	"github.com/ava-labs/avalanchego/vms/components/avax"
	"github.com/ava-labs/avalanchego/vms/platformvm/blocks"
	"github.com/ava-labs/avalanchego/vms/platformvm/status"
	"github.com/ava-labs/avalanchego/vms/platformvm/txs"
	"github.com/ava-labs/avalanchego/x/merkledb"
)

const (
	HistoryLength = int(256)    // from HyperSDK
	NodeCacheSize = int(65_536) // from HyperSDK

	utxoCacheSize = 8192 // from avax/utxo_state.go
)

var (
	_ State = (*merkleState)(nil)

	errNotYetImplemented = errors.New("not yet implemented")

	merkleStatePrefix      = []byte{0x0}
	merkleBlockPrefix      = []byte{0x1}
	merkleTxPrefix         = []byte{0x2}
	merkleIndexUTXOsPrefix = []byte{0x3} // to serve UTXOIDs(addr)
	merkleUptimesPrefix    = []byte{0x4}

	// merkle db sections
	metadataSectionPrefix      = []byte{0x0}
	merkleChainTimeKey         = append(metadataSectionPrefix, []byte{0x0}...)
	merkleLastAcceptedBlkIDKey = append(metadataSectionPrefix, []byte{0x1}...)
	merkleSuppliesPrefix       = append(metadataSectionPrefix, []byte{0x2}...)

	permissionedSubnetSectionPrefix = []byte{0x1}
	elasticSubnetSectionPrefix      = []byte{0x2}
	chainsSectionPrefix             = []byte{0x3}
	utxosSectionPrefix              = []byte{0x4}
	rewardUtxosSectionPrefix        = []byte{0x5}
)

func NewMerkleState(
	rawDB database.Database,
	metricsReg prometheus.Registerer,
) (Chain, error) {
	var (
		baseDB         = versiondb.New(rawDB)
		baseMerkleDB   = prefixdb.New(merkleStatePrefix, baseDB)
		blockDB        = prefixdb.New(merkleBlockPrefix, baseDB)
		txDB           = prefixdb.New(merkleTxPrefix, baseDB)
		indexedUTXOsDB = prefixdb.New(merkleIndexUTXOsPrefix, baseDB)
		localUptimesDB = prefixdb.New(merkleUptimesPrefix, baseDB)
	)

	ctx := context.TODO()
	noOpTracer, err := trace.New(trace.Config{Enabled: false})
	if err != nil {
		return nil, fmt.Errorf("failed creating noOpTraces: %w", err)
	}

	merkleDB, err := merkledb.New(ctx, baseMerkleDB, merkledb.Config{
		HistoryLength: HistoryLength,
		NodeCacheSize: NodeCacheSize,
		Reg:           prometheus.NewRegistry(),
		Tracer:        noOpTracer,
	})
	if err != nil {
		return nil, fmt.Errorf("failed creating merkleDB: %w", err)
	}

	rewardUTXOsCache, err := metercacher.New[ids.ID, []*avax.UTXO](
		"reward_utxos_cache",
		metricsReg,
		&cache.LRU[ids.ID, []*avax.UTXO]{Size: rewardUTXOsCacheSize},
	)
	if err != nil {
		return nil, err
	}

	suppliesCache, err := metercacher.New[ids.ID, *uint64](
		"supply_cache",
		metricsReg,
		&cache.LRU[ids.ID, *uint64]{Size: chainCacheSize},
	)
	if err != nil {
		return nil, err
	}

	transformedSubnetCache, err := metercacher.New(
		"transformed_subnet_cache",
		metricsReg,
		cache.NewSizedLRU[ids.ID, *txs.Tx](transformedSubnetTxCacheSize, txSize),
	)
	if err != nil {
		return nil, err
	}

	chainCache, err := metercacher.New[ids.ID, []*txs.Tx](
		"chain_cache",
		metricsReg,
		&cache.LRU[ids.ID, []*txs.Tx]{Size: chainCacheSize},
	)
	if err != nil {
		return nil, err
	}

	blockCache, err := metercacher.New[ids.ID, blocks.Block](
		"block_cache",
		metricsReg,
		cache.NewSizedLRU[ids.ID, blocks.Block](blockCacheSize, blockSize),
	)
	if err != nil {
		return nil, err
	}

	txCache, err := metercacher.New(
		"tx_cache",
		metricsReg,
		cache.NewSizedLRU[ids.ID, *txAndStatus](txCacheSize, txAndStatusSize),
	)
	if err != nil {
		return nil, err
	}

	res := &merkleState{
		baseDB:       baseDB,
		baseMerkleDB: baseMerkleDB,
		merkleDB:     merkleDB,

		currentStakers: newBaseStakers(),
		pendingStakers: newBaseStakers(),

		modifiedUTXOs:    make(map[ids.ID]*avax.UTXO),
		utxoCache:        &cache.LRU[ids.ID, *avax.UTXO]{Size: utxoCacheSize},
		addedRewardUTXOs: make(map[ids.ID][]*avax.UTXO),
		rewardUTXOsCache: rewardUTXOsCache,

		supplies:      make(map[ids.ID]uint64),
		suppliesCache: suppliesCache,

		addedPermissionedSubnets: make([]*txs.Tx, 0),
		permissionedSubnetCache:  make([]*txs.Tx, 0),
		addedElasticSubnets:      make(map[ids.ID]*txs.Tx),
		elasticSubnetCache:       transformedSubnetCache,

		addedChains: make(map[ids.ID][]*txs.Tx),
		chainCache:  chainCache,

		addedTxs: make(map[ids.ID]*txAndStatus),
		txCache:  txCache,
		txDB:     txDB,

		addedBlocks: make(map[ids.ID]blocks.Block),
		blockCache:  blockCache,
		blockDB:     blockDB,

		indexedUTXOsDB: indexedUTXOsDB,

		localUptimesCache:    make(map[ids.NodeID]map[ids.ID]*uptimes),
		modifiedLocalUptimes: make(map[ids.NodeID]set.Set[ids.ID]),
		localUptimesDB:       localUptimesDB,
	}
	return res, nil
}

type merkleState struct {
	baseDB       *versiondb.Database
	baseMerkleDB database.Database
	merkleDB     merkledb.MerkleDB // meklelized state

	// stakers section (missing Delegatee piece)
	// TODO: Consider moving delegatee to UTXOs section
	currentStakers *baseStakers
	pendingStakers *baseStakers

	// UTXOs section
	modifiedUTXOs map[ids.ID]*avax.UTXO            // map of UTXO ID -> *UTXO
	utxoCache     cache.Cacher[ids.ID, *avax.UTXO] // UTXO ID -> *UTXO. If the *UTXO is nil the UTXO doesn't exist

	addedRewardUTXOs map[ids.ID][]*avax.UTXO            // map of txID -> []*UTXO
	rewardUTXOsCache cache.Cacher[ids.ID, []*avax.UTXO] // txID -> []*UTXO

	// Metadata section
	chainTime          time.Time
	lastAcceptedBlkID  ids.ID
	lastAcceptedHeight uint64                        // Should this be written to state??
	supplies           map[ids.ID]uint64             // map of subnetID -> current supply
	suppliesCache      cache.Cacher[ids.ID, *uint64] // cache of subnetID -> current supply if the entry is nil, it is not in the database

	// Subnets section
	addedPermissionedSubnets []*txs.Tx                     // added SubnetTxs, waiting to be committed
	permissionedSubnetCache  []*txs.Tx                     // nil if the subnets haven't been loaded
	addedElasticSubnets      map[ids.ID]*txs.Tx            // map of subnetID -> transformSubnetTx
	elasticSubnetCache       cache.Cacher[ids.ID, *txs.Tx] // cache of subnetID -> transformSubnetTx if the entry is nil, it is not in the database

	// Chains section
	addedChains map[ids.ID][]*txs.Tx            // maps subnetID -> the newly added chains to the subnet
	chainCache  cache.Cacher[ids.ID, []*txs.Tx] // cache of subnetID -> the chains after all local modifications []*txs.Tx

	// Txs section
	// FIND a way to reduce use of these. No use in verification of addedTxs
	// a limited windows to support APIs
	addedTxs map[ids.ID]*txAndStatus            // map of txID -> {*txs.Tx, Status}
	txCache  cache.Cacher[ids.ID, *txAndStatus] // txID -> {*txs.Tx, Status}. If the entry is nil, it isn't in the database
	txDB     database.Database

	// Blocks section
	addedBlocks map[ids.ID]blocks.Block            // map of blockID -> Block
	blockCache  cache.Cacher[ids.ID, blocks.Block] // cache of blockID -> Block. If the entry is nil, it is not in the database
	blockDB     database.Database

	indexedUTXOsDB database.Database

	localUptimesCache    map[ids.NodeID]map[ids.ID]*uptimes // vdrID -> subnetID -> metadata
	modifiedLocalUptimes map[ids.NodeID]set.Set[ids.ID]     // vdrID -> subnetIDs
	localUptimesDB       database.Database
}

type uptimes struct {
	Duration    time.Duration `serialize:"true"`
	LastUpdated uint64        `serialize:"true"` // Unix time in seconds

	// txID        ids.ID // TODO ABENEGIA: is it needed by delegators and not validators?
	lastUpdated time.Time
}

// STAKERS section
func (ms *merkleState) GetCurrentValidator(subnetID ids.ID, nodeID ids.NodeID) (*Staker, error) {
	return ms.currentStakers.GetValidator(subnetID, nodeID)
}

func (ms *merkleState) PutCurrentValidator(staker *Staker) {
	ms.currentStakers.PutValidator(staker)
}

func (ms *merkleState) DeleteCurrentValidator(staker *Staker) {
	ms.currentStakers.DeleteValidator(staker)
}

func (ms *merkleState) GetCurrentDelegatorIterator(subnetID ids.ID, nodeID ids.NodeID) (StakerIterator, error) {
	return ms.currentStakers.GetDelegatorIterator(subnetID, nodeID), nil
}

func (ms *merkleState) PutCurrentDelegator(staker *Staker) {
	ms.currentStakers.PutDelegator(staker)
}

func (ms *merkleState) DeleteCurrentDelegator(staker *Staker) {
	ms.currentStakers.DeleteDelegator(staker)
}

func (ms *merkleState) GetCurrentStakerIterator() (StakerIterator, error) {
	return ms.currentStakers.GetStakerIterator(), nil
}

func (ms *merkleState) GetPendingValidator(subnetID ids.ID, nodeID ids.NodeID) (*Staker, error) {
	return ms.pendingStakers.GetValidator(subnetID, nodeID)
}

func (ms *merkleState) PutPendingValidator(staker *Staker) {
	ms.pendingStakers.PutValidator(staker)
}

func (ms *merkleState) DeletePendingValidator(staker *Staker) {
	ms.pendingStakers.DeleteValidator(staker)
}

func (ms *merkleState) GetPendingDelegatorIterator(subnetID ids.ID, nodeID ids.NodeID) (StakerIterator, error) {
	return ms.pendingStakers.GetDelegatorIterator(subnetID, nodeID), nil
}

func (ms *merkleState) PutPendingDelegator(staker *Staker) {
	ms.pendingStakers.PutDelegator(staker)
}

func (ms *merkleState) DeletePendingDelegator(staker *Staker) {
	ms.pendingStakers.DeleteDelegator(staker)
}

func (ms *merkleState) GetPendingStakerIterator() (StakerIterator, error) {
	return ms.pendingStakers.GetStakerIterator(), nil
}

func (*merkleState) GetDelegateeReward( /*subnetID*/ ids.ID /*vdrID*/, ids.NodeID) (amount uint64, err error) {
	return 0, errNotYetImplemented
}

func (*merkleState) SetDelegateeReward( /*subnetID*/ ids.ID /*vdrID*/, ids.NodeID /*amount*/, uint64) error {
	return errNotYetImplemented
}

// UTXOs section
func (ms *merkleState) GetUTXO(utxoID ids.ID) (*avax.UTXO, error) {
	if utxo, exists := ms.modifiedUTXOs[utxoID]; exists {
		if utxo == nil {
			return nil, database.ErrNotFound
		}
		return utxo, nil
	}
	if utxo, found := ms.utxoCache.Get(utxoID); found {
		if utxo == nil {
			return nil, database.ErrNotFound
		}
		return utxo, nil
	}

	key := make([]byte, 0, len(utxosSectionPrefix)+len(utxoID))
	copy(key, utxosSectionPrefix)
	key = append(key, utxoID[:]...)

	switch bytes, err := ms.merkleDB.Get(key); err {
	case nil:
		utxo := &avax.UTXO{}
		if _, err := txs.GenesisCodec.Unmarshal(bytes, utxo); err != nil {
			return nil, err
		}
		ms.utxoCache.Put(utxoID, utxo)
		return utxo, nil

	case database.ErrNotFound:
		ms.utxoCache.Put(utxoID, nil)
		return nil, database.ErrNotFound

	default:
		return nil, err
	}
}

func (ms *merkleState) UTXOIDs(addr []byte, start ids.ID, limit int) ([]ids.ID, error) {
	startKey := make([]byte, 0, len(addr)+len(start))
	copy(startKey, addr)
	startKey = append(startKey, start[:]...)

	iter := ms.indexedUTXOsDB.NewIteratorWithStart(startKey)
	defer iter.Release()

	utxoIDs := []ids.ID(nil)
	for len(utxoIDs) < limit && iter.Next() {
		utxoID, err := ids.ToID(iter.Key())
		if err != nil {
			return nil, err
		}
		if utxoID == start {
			continue
		}

		start = ids.Empty
		utxoIDs = append(utxoIDs, utxoID)
	}
	return utxoIDs, iter.Error()
}

func (ms *merkleState) AddUTXO(utxo *avax.UTXO) {
	ms.modifiedUTXOs[utxo.InputID()] = utxo
}

func (ms *merkleState) DeleteUTXO(utxoID ids.ID) {
	ms.modifiedUTXOs[utxoID] = nil
}

func (ms *merkleState) GetRewardUTXOs(txID ids.ID) ([]*avax.UTXO, error) {
	if utxos, exists := ms.addedRewardUTXOs[txID]; exists {
		return utxos, nil
	}
	if utxos, exists := ms.rewardUTXOsCache.Get(txID); exists {
		return utxos, nil
	}

	utxos := make([]*avax.UTXO, 0)

	prefix := make([]byte, 0, len(rewardUtxosSectionPrefix)+len(txID))
	copy(prefix, rewardUtxosSectionPrefix)
	prefix = append(prefix, txID[:]...)

	it := ms.merkleDB.NewIteratorWithPrefix(prefix)
	defer it.Release()

	for it.Next() {
		utxo := &avax.UTXO{}
		if _, err := txs.Codec.Unmarshal(it.Value(), utxo); err != nil {
			return nil, err
		}
		utxos = append(utxos, utxo)
	}
	if err := it.Error(); err != nil {
		return nil, err
	}

	ms.rewardUTXOsCache.Put(txID, utxos)
	return utxos, nil
}

func (ms *merkleState) AddRewardUTXO(txID ids.ID, utxo *avax.UTXO) {
	ms.addedRewardUTXOs[txID] = append(ms.addedRewardUTXOs[txID], utxo)
}

// METADATA Section
func (ms *merkleState) GetTimestamp() time.Time {
	return ms.chainTime
}

func (ms *merkleState) SetTimestamp(tm time.Time) {
	ms.chainTime = tm
}

func (ms *merkleState) GetLastAccepted() ids.ID {
	return ms.lastAcceptedBlkID
}

func (ms *merkleState) SetLastAccepted(lastAccepted ids.ID) {
	ms.lastAcceptedBlkID = lastAccepted
}

func (ms *merkleState) SetHeight(height uint64) {
	ms.lastAcceptedHeight = height
}

func (ms *merkleState) GetCurrentSupply(subnetID ids.ID) (uint64, error) {
	supply, ok := ms.supplies[subnetID]
	if ok {
		return supply, nil
	}
	cachedSupply, ok := ms.suppliesCache.Get(subnetID)
	if ok {
		if cachedSupply == nil {
			return 0, database.ErrNotFound
		}
		return *cachedSupply, nil
	}

	key := make([]byte, 0, len(merkleSuppliesPrefix)+len(subnetID[:]))
	copy(key, merkleSuppliesPrefix)
	key = append(key, subnetID[:]...)

	switch supplyBytes, err := ms.merkleDB.Get(key); err {
	case nil:
		supply, err := database.ParseUInt64(supplyBytes)
		if err != nil {
			return 0, fmt.Errorf("failed parsing supply: %w", err)
		}
		ms.suppliesCache.Put(subnetID, &supply)
		return supply, nil

	case database.ErrNotFound:
		ms.suppliesCache.Put(subnetID, nil)
		return 0, database.ErrNotFound

	default:
		return 0, err
	}
}

func (ms *merkleState) SetCurrentSupply(subnetID ids.ID, cs uint64) {
	ms.supplies[subnetID] = cs
}

// SUBNETS Section
func (ms *merkleState) GetSubnets() ([]*txs.Tx, error) {
	// Note: we want all subnets, so we don't look at addedSubnets
	// which are only part of them
	if ms.permissionedSubnetCache != nil {
		return ms.permissionedSubnetCache, nil
	}

	subnets := make([]*txs.Tx, 0)
	subnetDBIt := ms.merkleDB.NewIteratorWithPrefix(permissionedSubnetSectionPrefix)
	defer subnetDBIt.Release()

	for subnetDBIt.Next() {
		subnetTxBytes := subnetDBIt.Value()
		subnetTx, err := txs.Parse(txs.GenesisCodec, subnetTxBytes)
		if err != nil {
			return nil, err
		}
		subnets = append(subnets, subnetTx)
	}
	if err := subnetDBIt.Error(); err != nil {
		return nil, err
	}
	subnets = append(subnets, ms.addedPermissionedSubnets...)
	ms.permissionedSubnetCache = subnets
	return subnets, nil
}

func (ms *merkleState) AddSubnet(createSubnetTx *txs.Tx) {
	ms.addedPermissionedSubnets = append(ms.addedPermissionedSubnets, createSubnetTx)
}

func (ms *merkleState) GetSubnetTransformation(subnetID ids.ID) (*txs.Tx, error) {
	if tx, exists := ms.addedElasticSubnets[subnetID]; exists {
		return tx, nil
	}

	if tx, cached := ms.elasticSubnetCache.Get(subnetID); cached {
		if tx == nil {
			return nil, database.ErrNotFound
		}
		return tx, nil
	}

	subnetIDKey := make([]byte, 0, len(elasticSubnetSectionPrefix)+len(subnetID[:]))
	copy(subnetIDKey, merkleSuppliesPrefix)
	subnetIDKey = append(subnetIDKey, subnetID[:]...)

	transformSubnetTxID, err := database.GetID(ms.merkleDB, subnetIDKey)
	switch err {
	case nil:
		transformSubnetTx, _, err := ms.GetTx(transformSubnetTxID)
		if err != nil {
			return nil, err
		}
		ms.elasticSubnetCache.Put(subnetID, transformSubnetTx)
		return transformSubnetTx, nil

	case database.ErrNotFound:
		ms.elasticSubnetCache.Put(subnetID, nil)
		return nil, database.ErrNotFound

	default:
		return nil, err
	}
}

func (ms *merkleState) AddSubnetTransformation(transformSubnetTxIntf *txs.Tx) {
	transformSubnetTx := transformSubnetTxIntf.Unsigned.(*txs.TransformSubnetTx)
	ms.addedElasticSubnets[transformSubnetTx.Subnet] = transformSubnetTxIntf
}

// CHAINS Section
func (ms *merkleState) GetChains(subnetID ids.ID) ([]*txs.Tx, error) {
	if chains, cached := ms.chainCache.Get(subnetID); cached {
		return chains, nil
	}
	chains := make([]*txs.Tx, 0)

	prefix := make([]byte, 0, len(chainsSectionPrefix)+len(subnetID[:]))
	copy(prefix, chainsSectionPrefix)
	prefix = append(prefix, subnetID[:]...)

	chainDBIt := ms.merkleDB.NewIteratorWithPrefix(prefix)
	defer chainDBIt.Release()
	for chainDBIt.Next() {
		chainTxBytes := chainDBIt.Value()
		chainTx, err := txs.Parse(txs.GenesisCodec, chainTxBytes)
		if err != nil {
			return nil, err
		}
		chains = append(chains, chainTx)
	}
	if err := chainDBIt.Error(); err != nil {
		return nil, err
	}
	chains = append(chains, ms.addedChains[subnetID]...)
	ms.chainCache.Put(subnetID, chains)
	return chains, nil
}

func (ms *merkleState) AddChain(createChainTxIntf *txs.Tx) {
	createChainTx := createChainTxIntf.Unsigned.(*txs.CreateChainTx)
	subnetID := createChainTx.SubnetID

	ms.addedChains[subnetID] = append(ms.addedChains[subnetID], createChainTxIntf)
}

// TXs Section
func (ms *merkleState) GetTx(txID ids.ID) (*txs.Tx, status.Status, error) {
	if tx, exists := ms.addedTxs[txID]; exists {
		return tx.tx, tx.status, nil
	}
	if tx, cached := ms.txCache.Get(txID); cached {
		if tx == nil {
			return nil, status.Unknown, database.ErrNotFound
		}
		return tx.tx, tx.status, nil
	}

	txBytes, err := ms.txDB.Get(txID[:])
	switch err {
	case nil:
		stx := txBytesAndStatus{}
		if _, err := txs.GenesisCodec.Unmarshal(txBytes, &stx); err != nil {
			return nil, status.Unknown, err
		}

		tx, err := txs.Parse(txs.GenesisCodec, stx.Tx)
		if err != nil {
			return nil, status.Unknown, err
		}

		ptx := &txAndStatus{
			tx:     tx,
			status: stx.Status,
		}

		ms.txCache.Put(txID, ptx)
		return ptx.tx, ptx.status, nil

	case database.ErrNotFound:
		ms.txCache.Put(txID, nil)
		return nil, status.Unknown, database.ErrNotFound

	default:
		return nil, status.Unknown, err
	}
}

func (ms *merkleState) AddTx(tx *txs.Tx, status status.Status) {
	ms.addedTxs[tx.ID()] = &txAndStatus{
		tx:     tx,
		status: status,
	}
}

// BLOCKs Section
func (ms *merkleState) GetStatelessBlock(blockID ids.ID) (blocks.Block, error) {
	if blk, exists := ms.addedBlocks[blockID]; exists {
		return blk, nil
	}

	if blk, cached := ms.blockCache.Get(blockID); cached {
		if blk == nil {
			return nil, database.ErrNotFound
		}

		return blk, nil
	}

	blkBytes, err := ms.blockDB.Get(blockID[:])
	switch err {
	case nil:
		// Note: stored blocks are verified, so it's safe to unmarshal them with GenesisCodec
		blk, err := blocks.Parse(blocks.GenesisCodec, blkBytes)
		if err != nil {
			return nil, err
		}

		ms.blockCache.Put(blockID, blk)
		return blk, nil

	case database.ErrNotFound:
		ms.blockCache.Put(blockID, nil)
		return nil, database.ErrNotFound

	default:
		return nil, err
	}
}

func (ms *merkleState) AddStatelessBlock(block blocks.Block) {
	ms.addedBlocks[block.ID()] = block
}

// UPTIMES SECTION
func (ms *merkleState) GetUptime(vdrID ids.NodeID, subnetID ids.ID) (upDuration time.Duration, lastUpdated time.Time, err error) {
	nodeUptimes, exists := ms.localUptimesCache[vdrID]
	if !exists {
		return 0, time.Time{}, database.ErrNotFound
	}
	uptime, exists := nodeUptimes[subnetID]
	if !exists {
		return 0, time.Time{}, database.ErrNotFound
	}

	return uptime.Duration, uptime.lastUpdated, nil
}

func (ms *merkleState) SetUptime(vdrID ids.NodeID, subnetID ids.ID, upDuration time.Duration, lastUpdated time.Time) error {
	nodeUptimes, exists := ms.localUptimesCache[vdrID]
	if !exists {
		nodeUptimes = map[ids.ID]*uptimes{}
		ms.localUptimesCache[vdrID] = nodeUptimes
	}

	nodeUptimes[subnetID].Duration = upDuration
	nodeUptimes[subnetID].lastUpdated = lastUpdated

	// track diff
	updatedNodeUptimes, ok := ms.modifiedLocalUptimes[vdrID]
	if !ok {
		updatedNodeUptimes = set.Set[ids.ID]{}
		ms.modifiedLocalUptimes[vdrID] = updatedNodeUptimes
	}
	updatedNodeUptimes.Add(subnetID)
	return nil
}

func (ms *merkleState) GetStartTime(nodeID ids.NodeID, subnetID ids.ID) (time.Time, error) {
	staker, err := ms.currentStakers.GetValidator(subnetID, nodeID)
	if err != nil {
		return time.Time{}, err
	}
	return staker.StartTime, nil
}

// VALIDATORS Section
func (ms *merkleState) ValidatorSet(subnetID ids.ID, vdrs validators.Set) error {
	for nodeID, validator := range ms.currentStakers.validators[subnetID] {
		staker := validator.validator
		if err := vdrs.Add(nodeID, staker.PublicKey, staker.TxID, staker.Weight); err != nil {
			return err
		}

		delegatorIterator := NewTreeIterator(validator.delegators)
		for delegatorIterator.Next() {
			staker := delegatorIterator.Value()
			if err := vdrs.AddWeight(nodeID, staker.Weight); err != nil {
				delegatorIterator.Release()
				return err
			}
		}
		delegatorIterator.Release()
	}
	return nil
}

func (*merkleState) GetValidatorWeightDiffs( /*height*/ uint64 /*subnetID*/, ids.ID) (map[ids.NodeID]*ValidatorWeightDiff, error) {
	return nil, fmt.Errorf("MerkleDB GetValidatorWeightDiffs: %w", errNotYetImplemented)
}

func (*merkleState) GetValidatorPublicKeyDiffs( /*height*/ uint64) (map[ids.NodeID]*bls.PublicKey, error) {
	return nil, fmt.Errorf("MerkleDB GetValidatorPublicKeyDiffs: %w", errNotYetImplemented)
}

// DB Operations
func (ms *merkleState) Abort() {
	ms.baseDB.Abort()
}

func (ms *merkleState) Commit() error {
	defer ms.Abort()
	batch, err := ms.CommitBatch()
	if err != nil {
		return err
	}
	return batch.Write()
}

func (ms *merkleState) CommitBatch() (database.Batch, error) {
	// updateValidators is set to true here so that the validator manager is
	// kept up to date with the last accepted state.
	if err := ms.write(true /*updateValidators*/, ms.lastAcceptedHeight); err != nil {
		return nil, err
	}
	return ms.baseDB.CommitBatch()
}

func (*merkleState) Checksum() ids.ID {
	return ids.Empty
}

func (ms *merkleState) Close() error {
	errs := wrappers.Errs{}
	errs.Add(
		ms.localUptimesDB.Close(),
		ms.indexedUTXOsDB.Close(),
		ms.txDB.Close(),
		ms.blockDB.Close(),
		ms.merkleDB.Close(),
		ms.baseMerkleDB.Close(),
	)
	return errs.Err
}

func (ms *merkleState) write( /*updateValidators*/ bool /*height*/, uint64) error {
	errs := wrappers.Errs{}
	errs.Add(
		ms.writeMerkleState(),
		ms.writeBlocks(),
		ms.writeTXs(),
		ms.writelocalUptimes(),
	)
	return errs.Err
}

func (ms *merkleState) writeMerkleState() error {
	errs := wrappers.Errs{}
	view, err := ms.merkleDB.NewView()
	if err != nil {
		return err
	}

	ctx := context.TODO()
	errs.Add(
		ms.writeMetadata(view, ctx),
		ms.writePermissionedSubnets(view, ctx),
		ms.writeElasticSubnets(view, ctx),
		ms.writeChains(view, ctx),
		ms.writeUTXOs(view, ctx),
		ms.writeRewardUTXOs(view, ctx),
	)
	if errs.Err != nil {
		return err
	}

	return view.CommitToDB(ctx)
}

func (ms *merkleState) writeMetadata(view merkledb.TrieView, ctx context.Context) error {
	encodedChainTime, err := ms.chainTime.MarshalBinary()
	if err != nil {
		return fmt.Errorf("failed to encoding chainTime: %w", err)
	}
	if err := view.Insert(ctx, merkleChainTimeKey, encodedChainTime); err != nil {
		return fmt.Errorf("failed to write chainTime: %w", err)
	}

	if err := view.Insert(ctx, merkleLastAcceptedBlkIDKey, ms.lastAcceptedBlkID[:]); err != nil {
		return fmt.Errorf("failed to write last accepted: %w", err)
	}

	// lastAcceptedBlockHeight not persisted yet in merkleDB state.
	// TODO: Consider if it should be

	for subnetID, supply := range ms.supplies {
		supply := supply
		delete(ms.supplies, subnetID)
		ms.suppliesCache.Put(subnetID, &supply)

		key := make([]byte, 0, len(merkleSuppliesPrefix)+len(subnetID[:]))
		copy(key, merkleSuppliesPrefix)
		key = append(key, subnetID[:]...)
		if err := view.Insert(ctx, key, database.PackUInt64(supply)); err != nil {
			return fmt.Errorf("failed to write subnet %v supply: %w", subnetID, err)
		}
	}
	return nil
}

func (ms *merkleState) writePermissionedSubnets(view merkledb.TrieView, ctx context.Context) error {
	for _, subnetTx := range ms.addedPermissionedSubnets {
		subnetID := subnetTx.ID()

		key := make([]byte, 0, len(permissionedSubnetSectionPrefix)+len(subnetID[:]))
		copy(key, permissionedSubnetSectionPrefix)
		key = append(key, subnetID[:]...)

		if err := view.Insert(ctx, key, subnetTx.Bytes()); err != nil {
			return fmt.Errorf("failed to write subnetTx: %w", err)
		}
	}
	ms.addedPermissionedSubnets = nil
	return nil
}

func (ms *merkleState) writeElasticSubnets(view merkledb.TrieView, ctx context.Context) error {
	for _, subnetTx := range ms.addedElasticSubnets {
		subnetID := subnetTx.ID()

		key := make([]byte, 0, len(elasticSubnetSectionPrefix)+len(subnetID[:]))
		copy(key, elasticSubnetSectionPrefix)
		key = append(key, subnetID[:]...)

		if err := view.Insert(ctx, key, subnetTx.Bytes()); err != nil {
			return fmt.Errorf("failed to write subnetTx: %w", err)
		}
	}
	ms.addedElasticSubnets = nil
	return nil
}

func (ms *merkleState) writeChains(view merkledb.TrieView, ctx context.Context) error {
	for subnetID, chains := range ms.addedChains {
		prefixKey := make([]byte, 0, len(chainsSectionPrefix)+len(subnetID[:]))
		copy(prefixKey, chainsSectionPrefix)
		prefixKey = append(prefixKey, subnetID[:]...)

		for _, chainTx := range chains {
			chainID := chainTx.ID()

			key := make([]byte, 0, len(prefixKey)+len(chainID))
			copy(key, prefixKey)
			key = append(key, chainID[:]...)

			if err := view.Insert(ctx, key, chainTx.Bytes()); err != nil {
				return fmt.Errorf("failed to write chain: %w", err)
			}
		}
		delete(ms.addedChains, subnetID)
	}
	return nil
}

func (ms *merkleState) writeUTXOs(view merkledb.TrieView, ctx context.Context) error {
	for utxoID, utxo := range ms.modifiedUTXOs {
		delete(ms.modifiedUTXOs, utxoID)

		key := make([]byte, 0, len(utxosSectionPrefix)+len(utxoID))
		copy(key, utxosSectionPrefix)
		key = append(key, utxoID[:]...)

		if utxo == nil { // delete the UTXO
			switch _, err := ms.GetUTXO(utxoID); err {
			case nil:
				ms.utxoCache.Put(utxoID, nil)
				if err := view.Remove(ctx, key); err != nil {
					return err
				}

				// store the index
				if err := ms.writeUTXOsIndex(utxo, false /*insertUtxo*/); err != nil {
					return err
				}

			case database.ErrNotFound:
				return nil

			default:
				return err
			}
			continue
		}

		// insert the UTXO
		utxoBytes, err := txs.GenesisCodec.Marshal(txs.Version, utxo)
		if err != nil {
			return err
		}
		if err := view.Insert(ctx, key, utxoBytes); err != nil {
			return err
		}

		// store the index
		if err := ms.writeUTXOsIndex(utxo, true /*insertUtxo*/); err != nil {
			return err
		}
	}
	return nil
}

func (ms *merkleState) writeRewardUTXOs(view merkledb.TrieView, ctx context.Context) error {
	for txID, utxos := range ms.addedRewardUTXOs {
		delete(ms.addedRewardUTXOs, txID)
		ms.rewardUTXOsCache.Put(txID, utxos)

		prefix := make([]byte, 0, len(rewardUtxosSectionPrefix)+len(txID))
		copy(prefix, rewardUtxosSectionPrefix)
		prefix = append(prefix, txID[:]...)

		for _, utxo := range utxos {
			utxoID := utxo.InputID()

			key := make([]byte, 0, len(prefix)+len(utxoID))
			copy(key, prefix)
			key = append(key, utxoID[:]...)

			utxoBytes, err := txs.GenesisCodec.Marshal(txs.Version, utxo)
			if err != nil {
				return fmt.Errorf("failed to serialize reward UTXO: %w", err)
			}

			if err := view.Insert(ctx, key, utxoBytes); err != nil {
				return fmt.Errorf("failed to add reward UTXO: %w", err)
			}
		}
	}
	return nil
}

func (ms *merkleState) writeBlocks() error {
	for blkID, blk := range ms.addedBlocks {
		blkID := blkID

		delete(ms.addedBlocks, blkID)
		// Note: Evict is used rather than Put here because blk may end up
		// referencing additional data (because of shared byte slices) that
		// would not be properly accounted for in the cache sizing.
		ms.blockCache.Evict(blkID)

		if err := ms.blockDB.Put(blkID[:], blk.Bytes()); err != nil {
			return fmt.Errorf("failed to write block %s: %w", blkID, err)
		}
	}
	return nil
}

func (ms *merkleState) writeTXs() error {
	for txID, txStatus := range ms.addedTxs {
		txID := txID

		stx := txBytesAndStatus{
			Tx:     txStatus.tx.Bytes(),
			Status: txStatus.status,
		}

		// Note that we're serializing a [txBytesAndStatus] here, not a
		// *txs.Tx, so we don't use [txs.Codec].
		txBytes, err := txs.GenesisCodec.Marshal(txs.Version, &stx)
		if err != nil {
			return fmt.Errorf("failed to serialize tx: %w", err)
		}

		delete(ms.addedTxs, txID)
		// Note: Evict is used rather than Put here because stx may end up
		// referencing additional data (because of shared byte slices) that
		// would not be properly accounted for in the cache sizing.
		ms.txCache.Evict(txID)
		if err := ms.txDB.Put(txID[:], txBytes); err != nil {
			return fmt.Errorf("failed to add tx: %w", err)
		}
	}
	return nil
}

func (ms *merkleState) writeUTXOsIndex(utxo *avax.UTXO, insertUtxo bool) error {
	utxoID := utxo.InputID()
	addressable, ok := utxo.Out.(avax.Addressable)
	if !ok {
		return nil
	}
	addresses := addressable.Addresses()

	for _, addr := range addresses {
		key := make([]byte, 0, len(addr)+len(utxoID))
		copy(key, addr)
		key = append(key, utxoID[:]...)

		if insertUtxo {
			if err := ms.indexedUTXOsDB.Put(key, nil); err != nil {
				return err
			}
		} else {
			if err := ms.indexedUTXOsDB.Delete(key); err != nil {
				return err
			}
		}
	}
	return nil
}

func (ms *merkleState) writelocalUptimes() error {
	for vdrID, updatedSubnets := range ms.modifiedLocalUptimes {
		for subnetID := range updatedSubnets {
			key := make([]byte, 0, len(vdrID)+len(subnetID))
			copy(key, vdrID[:])
			key = append(key, subnetID[:]...)

			uptimes := ms.localUptimesCache[vdrID][subnetID]
			uptimes.LastUpdated = uint64(uptimes.lastUpdated.Unix())
			uptimeBytes, err := txs.GenesisCodec.Marshal(txs.Version, uptimes)
			if err != nil {
				return err
			}

			if err := ms.localUptimesDB.Put(key, uptimeBytes); err != nil {
				return fmt.Errorf("failed to add local uptimes: %w", err)
			}
		}
		delete(ms.modifiedLocalUptimes, vdrID)
	}
	return nil
}