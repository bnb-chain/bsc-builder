package miner

import (
	"errors"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/txpool"
	"github.com/ethereum/go-ethereum/core/types"
)

// commitWorkV2
// TODO(renee) take bundle pool status as LOOP_WAIT condition
func (w *worker) commitWorkV2(interruptCh chan int32, timestamp int64) {

}

// fillTransactions retrieves the pending bundles and transactions from the txpool and fills them
// into the given sealing block. The selection and ordering strategy can be extended in the future.
// TODO(renee) refer to flashbots/builder to optimize the bundle selection and ordering strategy
func (w *worker) fillTransactionsAndBundles(interruptCh chan int32, env *environment, stopTimer *time.Timer) (err error) {
	var (
		pending   map[common.Address][]*txpool.LazyTransaction
		localTxs  map[common.Address][]*txpool.LazyTransaction
		remoteTxs map[common.Address][]*txpool.LazyTransaction
		bundles   []*types.Bundle
	)
	{ // Split the pending transactions into locals and remotes
		// Fill the block with all available pending transactions.
		pending = w.eth.TxPool().Pending(false)

		localTxs, remoteTxs = make(map[common.Address][]*txpool.LazyTransaction), pending
		for _, account := range w.eth.TxPool().Locals() {
			if txs := remoteTxs[account]; len(txs) > 0 {
				delete(remoteTxs, account)
				localTxs[account] = txs
			}
		}

		bundles = w.eth.TxPool().PendingBundles(env.header.Number, env.header.Time)
	}

	var (
		bundleTxs    types.Transactions
		bundleProfit *big.Int
	)
	{
		bundleTxs, _, _ = w.generateOrderedBundles(env, bundles, pending)

		// log bundle merged status

		if len(bundleTxs) == 0 {
			return errors.New("no bundles to apply")
		}

		if err = w.commitBundle(env, bundleTxs, common.Address{}, interruptCh, stopTimer); err != nil {
			// log
			return err
		}

		// TODO(renee) bundleProfit = balance of coinbase
	}

	env.bundleProfit.Add(env.bundleProfit, bundleProfit)

	if len(localTxs) > 0 {
		txs := newTransactionsByPriceAndNonce(env.signer, localTxs, env.header.BaseFee)
		err = w.commitTransactions(env, txs, interruptCh, stopTimer)
		// we will abort here when:
		//   1.new block was imported
		//   2.out of Gas, no more transaction can be added.
		//   3.the mining timer has expired, stop adding transactions.
		//   4.interrupted resubmit timer, which is by default 10s.
		//     resubmit is for PoW only, can be deleted for PoS consensus later
		if err != nil {
			return err
		}
	}

	if len(remoteTxs) > 0 {
		txs := newTransactionsByPriceAndNonce(env.signer, remoteTxs, env.header.BaseFee)
		err = w.commitTransactions(env, txs, interruptCh, stopTimer)
	}

	// TODO(renee) blockReward = balance of coinbase
	var blockReward *big.Int
	env.blockReward.Add(env.blockReward, blockReward)

	return nil
}

func (w *worker) commitBundle(
	env *environment,
	txs types.Transactions,
	coinbase common.Address,
	interruptCh chan int32,
	stopTimer *time.Timer) error {
	//TODO implement me
	panic("implement me")
}

// generateOrderedBundles generates ordered txs from the given bundles.
// 1. sort bundle according to computed gas price when received.
// 2. simulate bundle based on the same state, resort.
// 3. merge resorted bundle based on the iterative state.
func (w *worker) generateOrderedBundles(
	env *environment,
	bundles []*types.Bundle,
	pendingTxs map[common.Address][]*txpool.LazyTransaction,
) (bundleTxs types.Transactions, simulatedBundle []*types.SimulatedBundle, err error) {
	//TODO implement me
	panic("implement me")
}

// mergeBundle merges the given bundle into the given environment.
// It returns the merged bundle and the number of transactions that were merged.
func (w *worker) mergeBundles(
	env *environment,
	bundles []*types.SimulatedBundle,
	pendingTxs map[common.Address]*types.Transaction) {
	ctx := &simContext{}

	for _, bundle := range bundles {
		w.simBundle(ctx, bundle.OriginalBundle)
	}

	//TODO implement me
	panic("implement me")
}

// simBundle computes the adjusted gas price for a whole bundle based on the specified state
// named computeBundleGas in flashbots
func (w *worker) simBundle(
	ctx *simContext,
	bundle *types.Bundle,
) (*types.SimulatedBundle, error) {
	//TODO implement me
	panic("implement me")
}

type simContext struct {
	index      int // bundle index in all bundles
	env        *environment
	state      *state.StateDB
	gasPool    *core.GasPool
	pendingTxs map[common.Address]*types.Transaction
}
