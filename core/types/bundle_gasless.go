package types

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

type SimulateGaslessBundleArgs struct {
	Txs []hexutil.Bytes `json:"txs"`
}

type GaslessTx struct {
	Hash    common.Hash
	GasUsed uint64
	GasFee  *big.Int
}

type SimulateGaslessBundleResp struct {
	ValidTxs         []GaslessTx
	BasedBlockNumber uint64
}
