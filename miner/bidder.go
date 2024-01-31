package miner

import (
	"context"
	"crypto/ecdsa"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
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
	SecretKey  string
}

type Bidder struct {
	config *BidderConfig
	engine consensus.Engine
	chain  *core.BlockChain

	validators map[common.Address]*ethclient.Client // validator address -> ethclient.Client

	bestWorksMu sync.RWMutex
	bestWorks   map[int64]*environment

	builderSecretKey *ecdsa.PrivateKey
}

func NewBidder(config *BidderConfig, engine consensus.Engine, chain *core.BlockChain) *Bidder {
	b := &Bidder{
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

	if config.SecretKey == "" {
		log.Error("Bidder: no secret key")
		return b
	}
	if config.SecretKey[:2] == "0x" {
		config.SecretKey = config.SecretKey[2:]
	}
	pk, err := crypto.HexToECDSA(config.SecretKey)
	if err != nil {
		log.Error("Bidder: invalid secret key", "err", err)
		return b
	}
	b.builderSecretKey = pk

	return b
}

// Bid called by go routine
// 1. ignore and return if bidder is disabled
// 2. panic if no valid validators
// 3. ignore and return if the work is not better than the current best work
// 4. update the bestWork and do bid
func (b *Bidder) Bid(work *environment) {
	if !b.config.Enable {
		log.Warn("Bidder: disabled")
		return
	}

	if len(b.validators) == 0 {
		log.Crit("Bidder: No valid validators")
		return
	}

	// if the work is not better than the current best work, ignore it
	if !b.isBestWork(work) {
		return
	}

	// update the bestWork and do bid
	b.setBestWork(work)
	b.bid(work)
}

func (b *Bidder) registered(validator common.Address) bool {
	_, ok := b.validators[validator]
	return ok
}

func (b *Bidder) register(validator common.Address, url string) error {
	if _, ok := b.validators[validator]; ok {
		return fmt.Errorf("validator %s already registered", validator.String())
	}

	cl, err := ethclient.DialOptions(context.Background(), url, rpc.WithHTTPClient(client))
	if err != nil {
		log.Error("Bidder: failed to dial validator", "url", url, "err", err)
		return err
	}

	b.validators[validator] = cl
	return nil
}

func (b *Bidder) unregister(validator common.Address) {
	if _, ok := b.validators[validator]; ok {
		delete(b.validators, validator)
	}
}

// bid notifies the next in-turn validator the work
// 1. compute the return profit for builder based on realtime traffic and validator commission
// 2. send bid to validator
func (b *Bidder) bid(work *environment) {
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
			Txs:         txs,
			// TODO: decide builderFee according to realtime traffic and validator commission
		}

		signature, err := b.signBid(&bid)
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

// isBestWork returns the work is better than the current best work
func (b *Bidder) isBestWork(work *environment) bool {
	if work.profit == nil {
		return false
	}

	return b.getBestWork(work.header.Number.Int64()).profit.Cmp(work.profit) < 0
}

// setBestWork sets the best work
func (b *Bidder) setBestWork(work *environment) {
	b.bestWorksMu.Lock()
	defer b.bestWorksMu.Unlock()

	b.bestWorks[work.header.Number.Int64()] = work
}

// getBestWork returns the best work
func (b *Bidder) getBestWork(blockNumber int64) *environment {
	b.bestWorksMu.RLock()
	defer b.bestWorksMu.RUnlock()

	return b.bestWorks[blockNumber]
}

// signBid signs the bid with builder's secret key
func (b *Bidder) signBid(bid *types.Bid) ([]byte, error) {
	bz, err := rlp.EncodeToBytes(bid)
	if err != nil {
		return nil, err
	}

	digestHash := crypto.Keccak256(bz)
	return crypto.Sign(digestHash, b.builderSecretKey)
}

// isEnabled returns whether the bid is enabled
func (b *Bidder) isEnabled() bool {
	return b.config.Enable
}
