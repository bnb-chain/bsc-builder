package miner

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/exp/slices"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
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
	Enable        bool
	Validators    []ValidatorConfig
	Account       common.Address
	DelayLeftOver time.Duration
}

type Bidder struct {
	config *BidderConfig
	engine consensus.Engine
	chain  *core.BlockChain

	validators map[common.Address]*ethclient.Client // validator address -> ethclient.Client

	works map[int64][]*environment

	newBidCh chan *environment
	exitCh   chan struct{}

	wg sync.WaitGroup

	wallet accounts.Wallet
}

func NewBidder(config *BidderConfig, engine consensus.Engine, chain *core.BlockChain) *Bidder {
	b := &Bidder{
		config:     config,
		engine:     engine,
		chain:      chain,
		validators: make(map[common.Address]*ethclient.Client),
		works:      make(map[int64][]*environment),
		newBidCh:   make(chan *environment, 10),
		exitCh:     make(chan struct{}),
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

func (b *Bidder) mainLoop() {
	defer b.wg.Done()

	timer := time.NewTimer(0)
	defer timer.Stop()
	<-timer.C // discard the initial tick

	currentHeight := b.chain.CurrentBlock().Number.Int64()
	for {
		select {
		case work := <-b.newBidCh:
			if work.header.Number.Int64() > currentHeight {
				currentHeight = work.header.Number.Int64()
				nextHeaderTimestamp := work.header.Time + b.chain.Config().Parlia.Period
				timer.Reset(time.Until(time.Unix(int64(nextHeaderTimestamp), 0).Add(-b.config.DelayLeftOver)))
			}
			b.works[work.header.Number.Int64()] = append(b.works[work.header.Number.Int64()], work)
		case <-timer.C:
			works := b.works[currentHeight]
			slices.SortStableFunc(works, func(i, j *environment) int {
				return j.profit.Cmp(i.profit)
			})

			for i, work := range works {
				// only bid for the top 3 most profitable works
				if i >= 3 {
					break
				}
				b.bid(work)
			}
		case <-b.exitCh:
			return
		}
	}
}

func (b *Bidder) SetWallet(wallet accounts.Wallet) {
	b.wallet = wallet
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

// signBid signs the bid with builder's account
func (b *Bidder) signBid(bid *types.Bid) ([]byte, error) {
	bz, err := rlp.EncodeToBytes(bid)
	if err != nil {
		return nil, err
	}

	return b.wallet.SignData(accounts.Account{Address: b.config.Account}, accounts.MimetypeTextPlain, bz)
}

// isEnabled returns whether the bid is enabled
func (b *Bidder) isEnabled() bool {
	return b.config.Enable
}
