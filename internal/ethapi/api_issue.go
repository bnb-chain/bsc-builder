package ethapi

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
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
	BidHash common.Hash `json:"bidHash"`
	Message string      `json:"message"`
}

func (s *IssueAPI) ReportIssue(ctx context.Context, args IssueArgs) error {
	log.Error("received issue", "bidHash", args.BidHash, "message", args.Message)

	// TODO(renee) consider not send bids to the validator for a while.
	// TODO(renee) track slash from validators in bidder.

	return nil
}
