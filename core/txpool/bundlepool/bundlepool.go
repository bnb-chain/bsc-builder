package bundlepool

import (
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/prque"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/txpool"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/params"
)

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

type Config struct {
	PriceLimit uint64 // Minimum gas price to enforce for acceptance into the pool
	PriceBump  uint64 // Minimum price bump percentage to replace an already existing transaction (nonce)

	GlobalQueue     uint64 // Maximum number of non-executable bundle slots for all accounts
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
}

func New(config Config, chain BlockChain) *BundlePool {
	return &BundlePool{
		bundleGasPricer: NewBundleGasPricer(config.BundleGasPricerExpireTime),
	}

}

func (p *BundlePool) Init(gasTip *big.Int, head *types.Header, reserve txpool.AddressReserver) error {
	//TODO implement me
	panic("implement me")
}

func (p *BundlePool) FilterBundle() bool {
	//TODO implement me
	panic("implement me")
}

func (p *BundlePool) AddBundle(bundle *types.Bundle) error {
	//TODO implement me
	panic("implement me")
}

func (p *BundlePool) PendingBundles(blockNumber *big.Int, blockTimestamp uint64) []*types.Bundle {
	p.mu.Lock()
	defer p.mu.Unlock()

	// returned values
	var ret []*types.Bundle
	// rolled over values
	bundles := make(map[common.Hash]*types.Bundle)

	for uid, bundle := range p.bundles {
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

		// return the ones which are in time
		ret = append(ret, bundle)
		// keep the bundles around internally until they need to be pruned
		bundles[uid] = bundle

		if bundle.Txs.Len() == 0 {
			continue
		}
	}

	p.bundles = bundles
	return ret
}

func (p *BundlePool) Filter(tx *types.Transaction) bool {
	return false
}

func (p *BundlePool) Close() error {
	//TODO implement me
	panic("implement me")
}

func (p *BundlePool) Reset(oldHead, newHead *types.Header) {
	//TODO implement me
	panic("implement me")
}

// SetGasTip updates the minimum price required by the subpool for a new
// transaction, and drops all transactions below this threshold.
func (p *BundlePool) SetGasTip(tip *big.Int) {
	//TODO implement me
	panic("implement me")
}

// Has returns an indicator whether subpool has a transaction cached with the
// given hash.
func (p *BundlePool) Has(hash common.Hash) bool {
	return false
}

// Get returns a transaction if it is contained in the pool, or nil otherwise.
func (p *BundlePool) Get(hash common.Hash) *txpool.Transaction {
	return nil
}

// Add enqueues a batch of transactions into the pool if they are valid. Due
// to the large transaction churn, add may postpone fully integrating the tx
// to a later point to batch multiple ones together.
func (p *BundlePool) Add(txs []*txpool.Transaction, local bool, sync bool) []error {
	return nil
}

// Pending retrieves all currently processable transactions, grouped by origin
// account and sorted by nonce.
func (p *BundlePool) Pending(enforceTips bool) map[common.Address][]*txpool.LazyTransaction {
	return nil
}

// SubscribeTransactions subscribes to new transaction events.
func (p *BundlePool) SubscribeTransactions(ch chan<- core.NewTxsEvent) event.Subscription {
	//TODO implement me
	panic("implement me")
}

// SubscribeReannoTxsEvent should return an event subscription of
// ReannoTxsEvent and send events to the given channel.
func (p *BundlePool) SubscribeReannoTxsEvent(chan<- core.ReannoTxsEvent) event.Subscription {
	//TODO implement me
	panic("implement me")
}

// Nonce returns the next nonce of an account, with all transactions executable
// by the pool already applied on top.
func (p *BundlePool) Nonce(addr common.Address) uint64 {
	//TODO implement me
	panic("implement me")
}

// Stats retrieves the current pool stats, namely the number of pending and the
// number of queued (non-executable) transactions.
func (p *BundlePool) Stats() (int, int) {
	//TODO implement me
	panic("implement me")
}

// Content retrieves the data content of the transaction pool, returning all the
// pending as well as queued transactions, grouped by account and sorted by nonce.
func (p *BundlePool) Content() (map[common.Address][]*types.Transaction, map[common.Address][]*types.Transaction) {
	//TODO implement me
	panic("implement me")
}

// ContentFrom retrieves the data content of the transaction pool, returning the
// pending as well as queued transactions of this address, grouped by nonce.
func (p *BundlePool) ContentFrom(addr common.Address) ([]*types.Transaction, []*types.Transaction) {
	//TODO implement me
	panic("implement me")
}

// Locals retrieves the accounts currently considered local by the pool.
func (p *BundlePool) Locals() []common.Address {
	//TODO implement me
	panic("implement me")
}

// Status returns the known status (unknown/pending/queued) of a transaction
// identified by their hashes.
func (p *BundlePool) Status(hash common.Hash) txpool.TxStatus {
	//TODO implement me
	panic("implement me")
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

// LatestGasPrice is a method to get latest cached gas price.
func (pool *BundleGasPricer) LatestGasPrice() *big.Int {
	pool.mu.RLock()
	defer pool.mu.RUnlock()
	return pool.latest
}

// MinimalGasPrice is a method to get minimal cached gas price.
func (pool *BundleGasPricer) MinimalGasPrice() *big.Int {
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
