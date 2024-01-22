package ethapi

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
)

const (
	MevNotRunningError = -38002
)

// IssueAPI offers an API for accepting bid issue from validator
type IssueAPI struct {
	b Backend
}

// NewIssueAPI creates a new bid issue API instance.
func NewIssueAPI(b Backend) *IssueAPI {
	return &IssueAPI{b}
}

// IssueArgs represents the arguments for a call.
type IssueArgs struct {
	Validator common.Address `json:"validator"`
	BidHash   common.Hash    `json:"bidHash"`
	Error     BidError       `json:"message"`
}

// BidError is an API error that encompasses an invalid bid with JSON error
// code and a binary data blob.
type BidError struct {
	error
	Code int
}

// ErrorCode returns the JSON error code for an invalid bid.
// See: https://github.com/ethereum/wiki/wiki/JSON-RPC-Error-Codes-Improvement-Proposal
func (e *BidError) ErrorCode() int {
	return e.Code
}

func (s *IssueAPI) ReportIssue(ctx context.Context, args IssueArgs) error {
	log.Error("received issue", "bidHash", args.BidHash, "message", args.Error.Error(), "code", args.Error.ErrorCode())

	switch respCode := args.Error.ErrorCode(); respCode {
	case MevNotRunningError:
		s.b.UnregisterMevValidator(args.Validator)
	}

	return nil
}
