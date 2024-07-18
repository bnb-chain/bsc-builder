package types

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

type SimulateGaslessBundleArgs struct {
	Txs []hexutil.Bytes `json:"txs"`
}

type GaslessTx struct {
	Index   int
	Hash    common.Hash
	GasUsed uint64
	Valid   bool
}

type SimulateGaslessBundleResp struct {
	Results          []GaslessTx
	BasedBlockNumber int64
}
