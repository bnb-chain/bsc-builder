package types

import (
	"math/big"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// TODO(roshan) refer to validator for all not in miner

// Bid represents a bid.
type Bid struct {
	BlockNumber uint64          `json:"blockNumber"`
	ParentHash  common.Hash     `json:"parentHash"`
	Txs         []hexutil.Bytes `json:"txs,omitempty"`
	GasUsed     uint64          `json:"gasUsed"`
	GasFee      uint64          `json:"gasFee"`
	Timestamp   int64           `json:"timestamp"`
	BuilderFee  *big.Int        `json:"builderFee"`

	// caches
	hash atomic.Value
}

func (b *Bid) Hash() common.Hash {
	if hash := b.hash.Load(); hash != nil {
		return hash.(common.Hash)
	}

	var h common.Hash
	h = rlpHash(b)

	b.hash.Store(h)
	return h
}

// BidArgs represents the arguments to submit a bid.
type BidArgs struct {
	// bid
	Bid *Bid
	// signed signature of the bid
	Signature string `json:"signature"`
}
