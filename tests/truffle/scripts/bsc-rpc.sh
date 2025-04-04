#!/usr/bin/env bash

DATA_DIR=/root/.ethereum

account_cnt=$(ls ${DATA_DIR}/keystore | wc -l)
i=1
unlock_sequences="0"
while [ "$i" -lt ${account_cnt} ]; do
    unlock_sequences="${unlock_sequences},${i}"
    i=$(( i + 1 ))
done 

geth --config ${DATA_DIR}/config.toml --datadir ${DATA_DIR} --netrestrict ${CLUSTER_CIDR} \
    --verbosity ${VERBOSE} --syncmode "full"\
    --rpc.allow-unprotected-txs --history.transactions 15768000 \
    -unlock ${unlock_sequences} --password /dev/null  >${DATA_DIR}/bscnode-rpc.log
