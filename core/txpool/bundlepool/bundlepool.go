package bundlepool

import (
	"errors"
	"math"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/prque"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/txpool"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
)

var (
	ErrSimulatorMissing   = errors.New("bundle simulator is missing")
	ErrBundleGasPriceLow  = errors.New("bundle gas price is too low")
	ErrBundleAlreadyExist = errors.New("bundle already exist")
	ErrBundlePoolOverflow = errors.New("bundle pool is full")
)

var bundleGauge = metrics.NewRegisteredGauge("txpool/bundles", nil)

// BlockChain defines the minimal set of methods needed to back a tx pool with
// a chain. Exists to allow mocking the live chain out of tests.
type BlockChain interface {
	// Config retrieves the chain's fork configuration.
	Config() *params.ChainConfig

	// CurrentBlock returns the current head of the chain.
	CurrentBlock() *types.Header

	// GetBlock retrieves a specific block, used during pool resets.
	GetBlock(hash common.Hash, number uint64) *types.Block

	// StateAt returns a state database for a given root hash (generally the head).
	StateAt(root common.Hash) (*state.StateDB, error)
}

type BundleSimulator interface {
	SimulateBundle(bundle *types.Bundle) (*big.Int, error)
}

type Config struct {
	PriceLimit uint64 // Minimum gas price to enforce for acceptance into the pool
	PriceBump  uint64 // Minimum price bump percentage to replace an already existing transaction (nonce)

	GlobalQueue     uint64 // Maximum number of non-executable bundle slots for all accounts
	BundleSlot      uint64 // Maximum number of bundle slots for all accounts
	MaxBundleBlocks uint64 // Maximum number of blocks for calculating MinimalBundleGasPrice

	BundleGasPricePercentile      uint8         // Percentile of the recent minimal mev gas price
	BundleGasPricerExpireTime     time.Duration // Store time duration amount of recent mev gas price
	UpdateBundleGasPricerInterval time.Duration // Time interval to update MevGasPricePool
}

type BundlePool struct {
	config Config
	chain  BlockChain
	mu     sync.RWMutex

	bundles         map[common.Hash]*types.Bundle
	bundleGasPricer *BundleGasPricer
	simulator       BundleSimulator
}

func New(config Config, chain BlockChain) *BundlePool {
	return &BundlePool{
		bundleGasPricer: NewBundleGasPricer(config.BundleGasPricerExpireTime),
	}
}

func (pool *BundlePool) SetBundleSimulator(simulator BundleSimulator) {
	pool.simulator = simulator
}

func (pool *BundlePool) Init(gasTip *big.Int, head *types.Header, reserve txpool.AddressReserver) error {
	// TODO implement me
	panic("implement me")
}

func (pool *BundlePool) FilterBundle(bundle *types.Bundle) bool {
	for _, tx := range bundle.Txs {
		if !pool.filter(tx) {
			return false
		}
	}
	return true
}

// AddBundle adds a mev bundle to the pool
func (pool *BundlePool) AddBundle(bundle *types.Bundle) error {
	if pool.simulator == nil {
		return ErrSimulatorMissing
	}

	bz, err := rlp.EncodeToBytes(bundle)
	if err != nil {
		return err
	}
	hash := crypto.Keccak256Hash(bz)
	bundle.Hash = hash

	price, err := pool.simulator.SimulateBundle(bundle)
	if err != nil {
		return err
	}
	minimalGasPrice := pool.bundleGasPricer.MinimalBundleGasPrice()
	if price.Cmp(minimalGasPrice) < 0 {
		return ErrBundleGasPriceLow
	}
	bundle.Price = price

	pool.mu.Lock()
	defer pool.mu.Unlock()
	if _, ok := pool.bundles[hash]; ok {
		return ErrBundleAlreadyExist
	}
	if len(pool.bundles) > int(pool.config.BundleSlot) {
		leastPrice := big.NewInt(math.MaxInt64)
		leastBundleHash := common.Hash{}
		for h, b := range pool.bundles {
			if b.Price.Cmp(leastPrice) < 0 {
				leastPrice = b.Price
				leastBundleHash = h
			}
		}
		if bundle.Price.Cmp(leastPrice) < 0 {
			return ErrBundlePoolOverflow
		}
		delete(pool.bundles, leastBundleHash)
	}
	pool.bundles[hash] = bundle

	bundleGauge.Update(int64(len(pool.bundles)))
	return nil
}

func (pool *BundlePool) PendingBundles(blockNumber *big.Int, blockTimestamp uint64) []*types.Bundle {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	// returned values
	var ret []*types.Bundle
	// rolled over values
	bundles := make(map[common.Hash]*types.Bundle)

	for uid, bundle := range pool.bundles {
		// Prune outdated bundles
		if (bundle.MaxTimestamp != 0 && blockTimestamp > bundle.MaxTimestamp) ||
			blockNumber.Cmp(big.NewInt(bundle.MaxBlockNumber)) > 0 {
			continue
		}

		// Roll over future bundles
		if bundle.MinTimestamp != 0 && blockTimestamp < bundle.MinTimestamp {
			bundles[uid] = bundle
			continue
		}

		// return the ones that are in time
		ret = append(ret, bundle)
		// keep the bundles around internally until they need to be pruned
		bundles[uid] = bundle

		if bundle.Txs.Len() == 0 {
			continue
		}
	}

	pool.bundles = bundles
	return ret
}

// AllBundles returns all the bundles currently in the pool
func (pool *BundlePool) AllBundles() []*types.Bundle {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	bundles := make([]*types.Bundle, 0, len(pool.bundles))
	for _, bundle := range pool.bundles {
		bundles = append(bundles, bundle)
	}
	return bundles
}

func (pool *BundlePool) Filter(tx *types.Transaction) bool {
	return false
}

func (pool *BundlePool) Close() error {
	// TODO implement me
	panic("implement me")
}

func (pool *BundlePool) Reset(oldHead, newHead *types.Header) {
	// TODO implement me
	panic("implement me")
}

// SetGasTip updates the minimum price required by the subpool for a new
// transaction, and drops all transactions below this threshold.
func (pool *BundlePool) SetGasTip(tip *big.Int) {
	// TODO implement me
	panic("implement me")
}

// Has returns an indicator whether subpool has a transaction cached with the
// given hash.
func (pool *BundlePool) Has(hash common.Hash) bool {
	return false
}

// Get returns a transaction if it is contained in the pool, or nil otherwise.
func (pool *BundlePool) Get(hash common.Hash) *txpool.Transaction {
	return nil
}

// Add enqueues a batch of transactions into the pool if they are valid. Due
// to the large transaction churn, add may postpone fully integrating the tx
// to a later point to batch multiple ones together.
func (pool *BundlePool) Add(txs []*txpool.Transaction, local bool, sync bool) []error {
	return nil
}

// Pending retrieves all currently processable transactions, grouped by origin
// account and sorted by nonce.
func (pool *BundlePool) Pending(enforceTips bool) map[common.Address][]*txpool.LazyTransaction {
	return nil
}

// SubscribeTransactions subscribes to new transaction events.
func (pool *BundlePool) SubscribeTransactions(ch chan<- core.NewTxsEvent) event.Subscription {
	// TODO implement me
	panic("implement me")
}

// SubscribeReannoTxsEvent should return an event subscription of
// ReannoTxsEvent and send events to the given channel.
func (pool *BundlePool) SubscribeReannoTxsEvent(chan<- core.ReannoTxsEvent) event.Subscription {
	// TODO implement me
	panic("implement me")
}

// Nonce returns the next nonce of an account, with all transactions executable
// by the pool already applied on top.
func (pool *BundlePool) Nonce(addr common.Address) uint64 {
	// TODO implement me
	panic("implement me")
}

// Stats retrieves the current pool stats, namely the number of pending and the
// number of queued (non-executable) transactions.
func (pool *BundlePool) Stats() (int, int) {
	// TODO implement me
	panic("implement me")
}

// Content retrieves the data content of the transaction pool, returning all the
// pending as well as queued transactions, grouped by account and sorted by nonce.
func (pool *BundlePool) Content() (map[common.Address][]*types.Transaction, map[common.Address][]*types.Transaction) {
	// TODO implement me
	panic("implement me")
}

// ContentFrom retrieves the data content of the transaction pool, returning the
// pending as well as queued transactions of this address, grouped by nonce.
func (pool *BundlePool) ContentFrom(addr common.Address) ([]*types.Transaction, []*types.Transaction) {
	// TODO implement me
	panic("implement me")
}

// Locals retrieves the accounts currently considered local by the pool.
func (pool *BundlePool) Locals() []common.Address {
	// TODO implement me
	panic("implement me")
}

// Status returns the known status (unknown/pending/queued) of a transaction
// identified by their hashes.
func (pool *BundlePool) Status(hash common.Hash) txpool.TxStatus {
	// TODO implement me
	panic("implement me")
}

func (pool *BundlePool) filter(tx *types.Transaction) bool {
	switch tx.Type() {
	case types.LegacyTxType, types.AccessListTxType, types.DynamicFeeTxType:
		return true
	default:
		return false
	}
}

// =====================================================================================================================

// NewBundleGasPricer creates a new BundleGasPricer.
func NewBundleGasPricer(expire time.Duration) *BundleGasPricer {
	return &BundleGasPricer{
		expire: expire,
		queue:  prque.New[int64, *gasPriceInfo](nil),
		latest: common.Big0,
	}
}

// BundleGasPricer is a limited number of queues.
// In order to avoid too drastic gas price changes, the latest n gas prices are cached.
// Allowed as long as the user's Gas Price matches this range.
type BundleGasPricer struct {
	mu     sync.RWMutex
	expire time.Duration
	queue  *prque.Prque[int64, *gasPriceInfo]
	latest *big.Int
}

type gasPriceInfo struct {
	val  *big.Int
	time time.Time
}

// Push is a method to cache a new gas price.
func (pool *BundleGasPricer) Push(gasPrice *big.Int) {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	pool.retire()
	index := -gasPrice.Int64()
	pool.queue.Push(&gasPriceInfo{val: gasPrice, time: time.Now()}, index)
	pool.latest = gasPrice
}

func (pool *BundleGasPricer) retire() {
	now := time.Now()
	for !pool.queue.Empty() {
		v, _ := pool.queue.Peek()
		info := v
		if info.time.Add(pool.expire).After(now) {
			break
		}
		pool.queue.Pop()
	}
}

// LatestBundleGasPrice is a method to get the latest-cached bundle gas price.
func (pool *BundleGasPricer) LatestBundleGasPrice() *big.Int {
	pool.mu.RLock()
	defer pool.mu.RUnlock()
	return pool.latest
}

// MinimalBundleGasPrice is a method to get minimal cached bundle gas price.
func (pool *BundleGasPricer) MinimalBundleGasPrice() *big.Int {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	if pool.queue.Empty() {
		return common.Big0
	}
	pool.retire()
	v, _ := pool.queue.Peek()
	return v.val
}

// Clear is a method to clear all caches.
func (pool *BundleGasPricer) Clear() {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	pool.queue.Reset()
}
