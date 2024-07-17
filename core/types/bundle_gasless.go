package types

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
)

type SimulateGaslessBundleArgs struct {
	Txs []hexutil.Bytes `json:"txs"`
}

type GaslessTx struct {
	Index   int
	GasUsed uint64
}

type SimulateGaslessBundleResp struct {
	ValidTxs         []GaslessTx
	BasedBlockNumber uint64
}
