package bundlepool

import (
	"container/heap"
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
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/ethereum/go-ethereum/params"
)

const (
	// TODO: decide on a good default value
	// bundleSlotSize is used to calculate how many data slots a single bundle
	// takes up based on its size. The slots are used as DoS protection, ensuring
	// that validating a new bundle remains a constant operation (in reality
	// O(maxslots), where max slots are 4 currently).
	bundleSlotSize = 128 * 1024 // 128KB
)

var (
	bundleGauge = metrics.NewRegisteredGauge("bundlepool/bundles", nil)
	slotsGauge  = metrics.NewRegisteredGauge("bundlepool/slots", nil)
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

type BundleSimulator interface {
	SimulateBundle(bundle *types.Bundle) (*big.Int, error)
}

type BundlePool struct {
	config Config

	bundles    map[common.Hash]*types.Bundle
	bundleHeap BundleHeap
	mu         sync.RWMutex

	slots uint64 // Number of slots currently allocated

	bundleGasPricer *BundleGasPricer
	simulator       BundleSimulator
}

func New(config Config) *BundlePool {
	// Sanitize the input to ensure no vulnerable gas prices are set
	config = (&config).sanitize()

	pool := &BundlePool{
		config:          config,
		bundles:         make(map[common.Hash]*types.Bundle),
		bundleHeap:      make(BundleHeap, 0),
		bundleGasPricer: NewBundleGasPricer(config.BundleGasPricerExpireTime),
	}

	return pool
}

func (p *BundlePool) SetBundleSimulator(simulator BundleSimulator) {
	p.simulator = simulator
}

func (p *BundlePool) Init(gasTip *big.Int, head *types.Header, reserve txpool.AddressReserver) error {
	return nil
}

func (p *BundlePool) FilterBundle(bundle *types.Bundle) bool {
	for _, tx := range bundle.Txs {
		if !p.filter(tx) {
			return false
		}
	}
	return true
}

// AddBundle adds a mev bundle to the pool
func (p *BundlePool) AddBundle(bundle *types.Bundle) error {
	if p.simulator == nil {
		return txpool.ErrSimulatorMissing
	}

	price, err := p.simulator.SimulateBundle(bundle)
	if err != nil {
		return err
	}
	minimalGasPrice := p.bundleGasPricer.MinimalBundleGasPrice()
	if price.Cmp(minimalGasPrice) < 0 && p.slots+numSlots(bundle) > p.config.GlobalSlots {
		return txpool.ErrBundleGasPriceLow
	}
	bundle.Price = price
	p.bundleGasPricer.Push(price)

	hash := bundle.Hash()
	if _, ok := p.bundles[hash]; ok {
		return txpool.ErrBundleAlreadyExist
	}
	for p.slots+numSlots(bundle) > p.config.GlobalSlots {
		p.drop()
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.bundles[hash] = bundle
	heap.Push(&p.bundleHeap, bundle)
	p.slots += numSlots(bundle)

	bundleGauge.Update(int64(len(p.bundles)))
	slotsGauge.Update(int64(p.slots))
	return nil
}

func (p *BundlePool) GetBundle(hash common.Hash) *types.Bundle {
	p.mu.RUnlock()
	defer p.mu.RUnlock()

	return p.bundles[hash]
}

func (p *BundlePool) PruneBundle(hash common.Hash) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.deleteBundle(hash)
}

func (p *BundlePool) PendingBundles(blockNumber *big.Int, blockTimestamp uint64) []*types.Bundle {
	p.mu.Lock()
	defer p.mu.Unlock()

	ret := make([]*types.Bundle, 0)
	for hash, bundle := range p.bundles {
		// Prune outdated bundles
		if (bundle.MaxTimestamp != 0 && blockTimestamp > bundle.MaxTimestamp) ||
			blockNumber.Cmp(new(big.Int).SetUint64(bundle.MaxBlockNumber)) > 0 {
			p.deleteBundle(hash)
			continue
		}

		// Roll over future bundles
		if bundle.MinTimestamp != 0 && blockTimestamp < bundle.MinTimestamp {
			continue
		}

		// return the ones that are in time
		ret = append(ret, bundle)
	}

	bundleGauge.Update(int64(len(p.bundles)))
	slotsGauge.Update(int64(p.slots))
	return ret
}

// AllBundles returns all the bundles currently in the pool
func (p *BundlePool) AllBundles() []*types.Bundle {
	p.mu.RUnlock()
	defer p.mu.RUnlock()
	bundles := make([]*types.Bundle, 0, len(p.bundles))
	for _, bundle := range p.bundles {
		bundles = append(bundles, bundle)
	}
	return bundles
}

func (p *BundlePool) Filter(tx *types.Transaction) bool {
	return false
}

func (p *BundlePool) Close() error {
	log.Info("Bundle pool stopped")
	return nil
}

func (p *BundlePool) Reset(oldHead, newHead *types.Header) {
	p.reset(newHead)
}

// SetGasTip updates the minimum price required by the subpool for a new
// transaction, and drops all transactions below this threshold.
func (p *BundlePool) SetGasTip(tip *big.Int) {
	return
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
	return nil
}

// SubscribeReannoTxsEvent should return an event subscription of
// ReannoTxsEvent and send events to the given channel.
func (p *BundlePool) SubscribeReannoTxsEvent(chan<- core.ReannoTxsEvent) event.Subscription {
	return nil
}

// Nonce returns the next nonce of an account, with all transactions executable
// by the pool already applied on topool.
func (p *BundlePool) Nonce(addr common.Address) uint64 {
	return 0
}

// Stats retrieves the current pool stats, namely the number of pending and the
// number of queued (non-executable) transactions.
func (p *BundlePool) Stats() (int, int) {
	// TODO implement me
	panic("implement me")
}

// Content retrieves the data content of the transaction pool, returning all the
// pending as well as queued transactions, grouped by account and sorted by nonce.
func (p *BundlePool) Content() (map[common.Address][]*types.Transaction, map[common.Address][]*types.Transaction) {
	// TODO implement me
	panic("implement me")
}

// ContentFrom retrieves the data content of the transaction pool, returning the
// pending as well as queued transactions of this address, grouped by nonce.
func (p *BundlePool) ContentFrom(addr common.Address) ([]*types.Transaction, []*types.Transaction) {
	// TODO implement me
	panic("implement me")
}

// Locals retrieves the accounts currently considered local by the pool.
func (p *BundlePool) Locals() []common.Address {
	return []common.Address{}
}

// Status returns the known status (unknown/pending/queued) of a transaction
// identified by their hashes.
func (p *BundlePool) Status(hash common.Hash) txpool.TxStatus {
	// TODO implement me
	panic("implement me")
}

func (p *BundlePool) filter(tx *types.Transaction) bool {
	switch tx.Type() {
	case types.LegacyTxType, types.AccessListTxType, types.DynamicFeeTxType:
		return true
	default:
		return false
	}
}

func (p *BundlePool) reset(newHead *types.Header) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Prune outdated bundles
	for hash, bundle := range p.bundles {
		if (bundle.MaxTimestamp != 0 && newHead.Time > bundle.MaxTimestamp) ||
			newHead.Number.Cmp(new(big.Int).SetUint64(bundle.MaxBlockNumber)) > 0 {
			p.slots -= numSlots(p.bundles[hash])
			delete(p.bundles, hash)
		}
	}
}

// deleteBundle deletes a bundle from the pool.
// It assumes that the caller holds the pool's lock.
func (p *BundlePool) deleteBundle(hash common.Hash) {
	p.slots -= numSlots(p.bundles[hash])
	delete(p.bundles, hash)
}

// drop removes the bundle with the lowest gas price from the pool.
func (p *BundlePool) drop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for len(p.bundleHeap) > 0 {
		// Pop the bundle with the lowest gas price
		// the min element in the heap may not exist in the pool as it may be pruned
		leastPriceBundleHash := heap.Pop(&p.bundleHeap).(*types.Bundle).Hash()
		if _, ok := p.bundles[leastPriceBundleHash]; ok {
			p.deleteBundle(leastPriceBundleHash)
			break
		}
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
func (bgp *BundleGasPricer) Push(gasPrice *big.Int) {
	bgp.mu.Lock()
	defer bgp.mu.Unlock()
	bgp.retire()
	index := -gasPrice.Int64()
	bgp.queue.Push(&gasPriceInfo{val: gasPrice, time: time.Now()}, index)
	bgp.latest = gasPrice
}

func (bgp *BundleGasPricer) retire() {
	now := time.Now()
	for !bgp.queue.Empty() {
		v, _ := bgp.queue.Peek()
		info := v
		if info.time.Add(bgp.expire).After(now) {
			break
		}
		bgp.queue.Pop()
	}
}

// LatestBundleGasPrice is a method to get the latest-cached bundle gas price.
func (bgp *BundleGasPricer) LatestBundleGasPrice() *big.Int {
	bgp.mu.RLock()
	defer bgp.mu.RUnlock()
	return bgp.latest
}

// MinimalBundleGasPrice is a method to get minimal cached bundle gas price.
func (bgp *BundleGasPricer) MinimalBundleGasPrice() *big.Int {
	bgp.mu.Lock()
	defer bgp.mu.Unlock()
	if bgp.queue.Empty() {
		return common.Big0
	}
	bgp.retire()
	v, _ := bgp.queue.Peek()
	return v.val
}

// Clear is a method to clear all caches.
func (bgp *BundleGasPricer) Clear() {
	bgp.mu.Lock()
	defer bgp.mu.Unlock()
	bgp.queue.Reset()
}

// numSlots calculates the number of slots needed for a single bundle.
func numSlots(bundle *types.Bundle) uint64 {
	return (bundle.Size() + bundleSlotSize - 1) / bundleSlotSize
}

// =====================================================================================================================

type BundleHeap []*types.Bundle

func (h *BundleHeap) Len() int { return len(*h) }

func (h *BundleHeap) Less(i, j int) bool {
	return (*h)[i].Price.Cmp((*h)[j].Price) == -1
}

func (h *BundleHeap) Swap(i, j int) { (*h)[i], (*h)[j] = (*h)[j], (*h)[i] }

func (h *BundleHeap) Push(x interface{}) {
	*h = append(*h, x.(*types.Bundle))
}

func (h *BundleHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}
