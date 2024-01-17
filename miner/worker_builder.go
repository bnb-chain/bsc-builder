package miner

import (
	"errors"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/txpool"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

var (
	errSimulateReceiptFailed = errors.New("simulate receipt failed")
	errBundlePriceTooLow     = errors.New("bundle price too low")
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
	stopTimer *time.Timer,
) error {
	// TODO implement me
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
	// TODO implement me
	panic("implement me")
}

func (w *worker) simulateBundles(env *environment, bundles []types.Bundle, pendingTxs map[common.Address]types.Transactions) ([]types.SimulatedBundle, error) {
	headerHash := env.header.Hash()
	simCache := w.bundleCache.GetBundleCache(headerHash)

	simResult := make([]*types.SimulatedBundle, len(bundles))

	var wg sync.WaitGroup
	for i, bundle := range bundles {
		if simmed, ok := simCache.GetSimulatedBundle(bundle.Hash()); ok {
			simResult = append(simResult, simmed)
			continue
		}

		wg.Add(1)
		go func(idx int, bundle types.Bundle, state *state.StateDB) {
			defer wg.Done()

			if len(bundle.Txs) == 0 {
				return
			}
			gasPool := prepareGasPool(env.header.GasLimit)
			simmed, err := w.simBundle(env, bundle, state, gasPool, pendingTxs, 0, true, true)
			if err != nil {
				log.Trace("Error computing gas for a bundle", "error", err)
				return
			}
			simResult[idx] = &simmed
		}(i, bundle, env.state.Copy())
	}

	wg.Wait()

	simCache.UpdateSimulatedBundles(simResult, bundles)
	simulatedBundles := make([]types.SimulatedBundle, 0, len(bundles))
	for _, bundle := range simResult {
		if bundle != nil {
			simulatedBundles = append(simulatedBundles, *bundle)
			if len(simulatedBundles) >= w.config.MaxSimulateBundles {
				break
			}
		}
	}

	return simulatedBundles, nil
}

// mergeBundle merges the given bundle into the given environment.
// It returns the merged bundle and the number of transactions that were merged.
func (w *worker) mergeBundles(
	env *environment,
	bundles []*types.SimulatedBundle,
	pendingTxs map[common.Address]types.Transactions,
) (types.Transactions, types.SimulatedBundle, int, error) {
	currentState := env.state.Copy()
	gasPool := prepareGasPool(env.header.GasLimit)

	finalBundleTxs := types.Transactions{}
	mergedBundle := types.SimulatedBundle{
		TotalEth:          new(big.Int),
		EthSentToCoinbase: new(big.Int),
	}

	count := 0
	for _, bundle := range bundles {
		prevState := currentState.Copy()
		prevGasPool := new(core.GasPool).AddGas(gasPool.Gas())

		// the floor gas price is 99/100 what was simulated at the top of the block
		floorGasPrice := new(big.Int).Mul(bundle.MevGasPrice, big.NewInt(99))
		floorGasPrice = floorGasPrice.Div(floorGasPrice, big.NewInt(100))

		simmed, err := w.simBundle(env, bundle.OriginalBundle, currentState, gasPool, pendingTxs, len(finalBundleTxs), true, false)
		if err != nil || simmed.MevGasPrice.Cmp(floorGasPrice) <= 0 {
			currentState = prevState
			gasPool = prevGasPool
			log.Error("Failed to merge one bundle", "err", err, "floorGasPrice", floorGasPrice)
			continue
		}

		log.Info("Included bundle", "ethToCoinbase", ethIntToFloat(simmed.TotalEth), "gasUsed", simmed.TotalGasUsed, "bundleScore", simmed.MevGasPrice, "bundleLength", len(simmed.OriginalBundle.Txs))

		finalBundleTxs = append(finalBundleTxs, bundle.OriginalBundle.Txs...)
		mergedBundle.TotalEth.Add(mergedBundle.TotalEth, simmed.TotalEth)
		mergedBundle.EthSentToCoinbase.Add(mergedBundle.EthSentToCoinbase, simmed.EthSentToCoinbase)
		mergedBundle.TotalGasUsed += simmed.TotalGasUsed
		count++
	}

	if len(finalBundleTxs) == 0 {
		return nil, types.SimulatedBundle{}, count, nil
	}

	return finalBundleTxs, types.SimulatedBundle{
		MevGasPrice:       new(big.Int).Div(mergedBundle.TotalEth, new(big.Int).SetUint64(mergedBundle.TotalGasUsed)),
		TotalEth:          mergedBundle.TotalEth,
		EthSentToCoinbase: mergedBundle.EthSentToCoinbase,
		TotalGasUsed:      mergedBundle.TotalGasUsed,
	}, count, nil
}

// simBundle computes the adjusted gas price for a whole bundle based on the specified state
// named computeBundleGas in flashbots
func (w *worker) simBundle(
	env *environment, bundle types.Bundle, state *state.StateDB, gasPool *core.GasPool,
	pendingTxs map[common.Address]types.Transactions, currentTxCount int,
	prune, pruneGasExceed bool,
) (types.SimulatedBundle, error) {
	var (
		totalGasUsed uint64
		tempGasUsed  uint64
	)
	gasFees := new(big.Int)
	ethSentToCoinbase := new(big.Int)
	for i, tx := range bundle.Txs {
		state.SetTxContext(tx.Hash(), i+currentTxCount)
		coinbaseBalanceBefore := state.GetBalance(w.coinbase)

		receipt, err := core.ApplyTransaction(w.chainConfig, w.chain, &w.coinbase, gasPool, state, env.header, tx, &tempGasUsed, *w.chain.GetVMConfig())
		if err != nil {
			if prune {
				if errors.Is(err, core.ErrGasLimitReached) && !pruneGasExceed {
					log.Warn("bundle gas limit exceed", "hash", bundle.Hash().String(), "err", err)
				} else {
					log.Warn("Prune bundle because of err", "hash", bundle.Hash().String(), "err", err)
					w.eth.TxPool().PruneBundle(bundle.Hash())
				}
			}
			return types.SimulatedBundle{}, err
		}
		if receipt.Status == types.ReceiptStatusFailed && !containsHash(bundle.RevertingTxHashes, receipt.TxHash) {
			if prune {
				log.Warn("Prune bundle because of failed tx", "hash", bundle.Hash().String())

				w.eth.TxPool().PruneBundle(bundle.Hash())
			}
			return types.SimulatedBundle{}, errSimulateReceiptFailed
		}

		totalGasUsed += receipt.GasUsed
		from, err := types.Sender(env.signer, tx)
		if err != nil {
			return types.SimulatedBundle{}, err
		}

		txInPendingPool := false
		if accountTxs, ok := pendingTxs[from]; ok {
			// check if tx is in pending pool
			txNonce := tx.Nonce()

			for _, accountTx := range accountTxs {
				if accountTx.Nonce() == txNonce {
					txInPendingPool = true
					break
				}
			}
		}

		gasUsed := new(big.Int).SetUint64(receipt.GasUsed)
		gasFeesTx := gasUsed.Mul(gasUsed, tx.GasPrice())
		coinbaseBalanceAfter := state.GetBalance(w.coinbase)
		coinbaseDelta := big.NewInt(0).Sub(coinbaseBalanceAfter, coinbaseBalanceBefore)
		ethSentToCoinbase.Add(ethSentToCoinbase, coinbaseDelta)

		if !txInPendingPool {
			// If tx is not in pending pool, count the gas fees
			gasFees.Add(gasFees, gasFeesTx)
		}
	}

	totalEth := new(big.Int).Add(ethSentToCoinbase, gasFees)

	mevGasPrice := new(big.Int).Div(totalEth, new(big.Int).SetUint64(totalGasUsed))
	if mevGasPrice.Cmp(big.NewInt(w.config.MevGasPriceFloor)) < 0 {
		if prune {
			log.Warn("Prune bundle because of not enough gas price", "hash", bundle.Hash().String())
			w.eth.TxPool().PruneBundle(bundle.Hash())
		}
		return types.SimulatedBundle{}, errBundlePriceTooLow
	}

	return types.SimulatedBundle{
		MevGasPrice:       mevGasPrice,
		TotalEth:          totalEth,
		EthSentToCoinbase: ethSentToCoinbase,
		TotalGasUsed:      totalGasUsed,
		OriginalBundle:    bundle,
	}, nil
}

func containsHash(arr []common.Hash, match common.Hash) bool {
	for _, elem := range arr {
		if elem == match {
			return true
		}
	}
	return false
}

// ethIntToFloat is for formatting a big.Int in wei to eth
func ethIntToFloat(eth *big.Int) *big.Float {
	if eth == nil {
		return big.NewFloat(0)
	}
	return new(big.Float).Quo(new(big.Float).SetInt(eth), new(big.Float).SetInt(big.NewInt(params.Ether)))
}

func prepareGasPool(gasLimit uint64) *core.GasPool {
	gasPool := new(core.GasPool).AddGas(gasLimit)
	gasPool.SubGas(params.SystemTxsGas * 3) // reserve gas for system txs(keep align with mainnet)
	return gasPool
}
