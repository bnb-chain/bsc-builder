package types

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

type Bundle struct {
	Txs               Transactions
	MaxBlockNumber    int64
	MinTimestamp      uint64
	MaxTimestamp      uint64
	RevertingTxHashes []common.Hash

	Hash  common.Hash `rlp:"-"`
	Price *big.Int    // for bundle compare and prune
}

type SimulatedBundle struct {
	// TODO(renee) not export

	MevGasPrice       *big.Int
	TotalEth          *big.Int
	EthSentToCoinbase *big.Int
	TotalGasUsed      uint64
	OriginalBundle    *Bundle
}
