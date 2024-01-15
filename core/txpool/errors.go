// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package txpool

import "errors"

var (
	// ErrAlreadyKnown is returned if the transaction is already contained
	// within the pool.
	ErrAlreadyKnown = errors.New("already known")

	// ErrInvalidSender is returned if the transaction contains an invalid signature.
	ErrInvalidSender = errors.New("invalid sender")

	// ErrUnderpriced is returned if a transaction's gas price is below the minimum
	// configured for the transaction pool.
	ErrUnderpriced = errors.New("transaction underpriced")

	// ErrReplaceUnderpriced is returned if a transaction is attempted to be replaced
	// with a different one without the required price bump.
	ErrReplaceUnderpriced = errors.New("replacement transaction underpriced")

	// ErrAccountLimitExceeded is returned if a transaction would exceed the number
	// allowed by a pool for a single account.
	ErrAccountLimitExceeded = errors.New("account limit exceeded")

	// ErrGasLimit is returned if a transaction's requested gas limit exceeds the
	// maximum allowance of the current block.
	ErrGasLimit = errors.New("exceeds block gas limit")

	// ErrNegativeValue is a sanity error to ensure no one is able to specify a
	// transaction with a negative value.
	ErrNegativeValue = errors.New("negative value")

	// ErrOversizedData is returned if the input data of a transaction is greater
	// than some meaningful limit a user might use. This is not a consensus error
	// making the transaction invalid, rather a DOS protection.
	ErrOversizedData = errors.New("oversized data")

	// ErrFutureReplacePending is returned if a future transaction replaces a pending
	// transaction. Future transactions should only be able to replace other future transactions.
	ErrFutureReplacePending = errors.New("future transaction tries to replace pending")

	// ErrInBlackList is returned if the sender or to is in black list.
	ErrInBlackList = errors.New("sender or to in black list")

	// ErrTxPoolOverflow is returned if the transaction pool is full and can't accept
	// another remote transaction.
	ErrTxPoolOverflow = errors.New("txpool is full")

	// ErrNoActiveJournal is returned if a transaction is attempted to be inserted
	// into the journal, but no such file is currently open.
	ErrNoActiveJournal = errors.New("no active journal")

	// ErrSimulatorMissing is returned if the bundle simulator is missing.
	ErrSimulatorMissing = errors.New("bundle simulator is missing")

	// ErrBundleGasPriceLow is returned if the bundle gas price is too low.
	ErrBundleGasPriceLow = errors.New("bundle gas price is too low")

	// ErrBundleAlreadyExist is returned if the bundle is already contained
	// within the pool.
	ErrBundleAlreadyExist = errors.New("bundle already exist")

	// ErrBundlePoolOverflow is returned if the bundle pool is full
	ErrBundlePoolOverflow = errors.New("bundle pool is full")
)
