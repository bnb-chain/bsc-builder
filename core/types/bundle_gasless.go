package types

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common/hexutil"
)

type SimulateGaslessBundleArgs struct {
	Txs []hexutil.Bytes `json:"txs"`
}

type GaslessTx struct {
	Index   int
	GasUsed uint64
	GasFee  *big.Int
}

type SimulateGaslessBundleResp struct {
	ValidTxs         []GaslessTx
	BasedBlockNumber uint64
}
