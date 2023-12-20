package miner

import (
	"context"
	"crypto/tls"
	"math/big"
	"net"
	"net/http"
	"sync"
	"time"

	jsoniter "github.com/json-iterator/go"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"
)

var (
	dialer = &net.Dialer{
		Timeout:   time.Second,
		KeepAlive: 60 * time.Second,
	}

	transport = &http.Transport{
		DialContext:         dialer.DialContext,
		MaxIdleConnsPerHost: 50,
		MaxConnsPerHost:     50,
		IdleConnTimeout:     90 * time.Second,
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
	}

	client = &http.Client{
		Timeout:   5 * time.Second,
		Transport: transport,
	}
)

type ValidatorConfig struct {
	Address common.Address
	URL     string
}

type BidderConfig struct {
	Enable     bool
	Validators []ValidatorConfig
}

type bidder struct {
	config *BidderConfig
	engine consensus.Engine
	chain  *core.BlockChain

	validators map[common.Address]*ethclient.Client // validator address -> ethclient.Client

	bestWorksMu sync.RWMutex
	bestWorks   map[int64]*environment
}

func NewBidder(config *BidderConfig, engine consensus.Engine, chain *core.BlockChain) *bidder {
	b := &bidder{
		config:     config,
		engine:     engine,
		chain:      chain,
		validators: make(map[common.Address]*ethclient.Client),
		bestWorks:  make(map[int64]*environment),
	}

	if !config.Enable {
		return b
	}

	for _, v := range config.Validators {
		cl, err := ethclient.DialOptions(context.Background(), v.URL, rpc.WithHTTPClient(client))
		if err != nil {
			log.Error("Bidder: failed to dial validator", "url", v.URL, "err", err)
			continue
		}

		b.validators[v.Address] = cl
	}

	if len(b.validators) == 0 {
		log.Warn("Bidder: No valid validators")
	}

	return b
}

// Bid called by go routine
// 1. ignore and return if bidder is disabled
// 2. panic if no valid validators
// 3. ignore and return if the work is not better than the current best work
// 4. update the bestWork and do bid
func (b *bidder) Bid(work *environment) {
	if !b.config.Enable {
		log.Warn("Bidder: disabled")
		return
	}

	if len(b.validators) == 0 {
		log.Crit("Bidder: No valid validators")
		return
	}

	// if the work is not better than the current best work, ignore it
	if !b.winBestWork(work) {
		return
	}

	// update the bestWork and do bid
	b.setBestWork(work)
	b.bid(work)
}

func (b *bidder) registered(validator common.Address) bool {
	_, ok := b.validators[validator]
	return ok
}

// bid notifies the next in-turn validator the work
// 1. compute the return profit for builder based on realtime traffic and validator commission
// 2. send bid to validator
func (b *bidder) bid(work *environment) {
	var (
		cli     = b.validators[work.coinbase]
		parent  = b.chain.CurrentBlock()
		bidArgs *types.BidArgs
	)

	if cli == nil {
		log.Error("Bidder: invalid validator", "validator", work.coinbase)
		return
	}

	// construct bid from work
	{
		var txs []hexutil.Bytes
		for _, tx := range work.txs {
			var txBytes []byte
			var err error
			txBytes, err = tx.MarshalBinary()
			if err != nil {
				log.Error("Bidder: fail to marshal tx", "tx", tx, "err", err)
				return
			}
			txs = append(txs, txBytes)
		}

		bid := types.Bid{
			BlockNumber: parent.Number.Uint64() + 1,
			ParentHash:  parent.Hash(),
			GasUsed:     work.header.GasUsed,
			GasFee:      work.blockReward.Uint64(),
			// TODO(renee) decide builderFee according to realtime traffic and validator commission
			BuilderFee: big.NewInt(int64(float64(work.bundleProfit.Uint64() * 5 / 100))),
			Txs:        txs,
			Timestamp:  time.Now().Unix(),
		}

		// TODO(roshan) review sign
		data, _ := jsoniter.Marshal(bid)
		signature, err := b.engine.SealData(data)

		if err != nil {
			log.Error("Bidder: fail to sign bid", "err", err)
			return
		}

		bidArgs = &types.BidArgs{
			Bid:       &bid,
			Signature: hexutil.Encode(signature),
		}
	}

	err := cli.BidBlock(context.Background(), bidArgs)
	if err != nil {
		log.Error("Bidder: bidding failed", "err", err)
		return
	}

	log.Debug("Bidder: bidding success")

	return
}

// winBestWork returns the work is better than the current best work
func (b *bidder) winBestWork(work *environment) bool {
	return b.getBestWork(work.header.Number.Int64()).blockReward.Cmp(work.blockReward) < 0
}

// setBestWork sets the best work
func (b *bidder) setBestWork(work *environment) {
	b.bestWorksMu.Lock()
	defer b.bestWorksMu.Unlock()

	b.bestWorks[work.header.Number.Int64()] = work
}

// getBestWork returns the best work
func (b *bidder) getBestWork(blockNumber int64) *environment {
	b.bestWorksMu.RLock()
	defer b.bestWorksMu.RUnlock()

	return b.bestWorks[blockNumber]
}
