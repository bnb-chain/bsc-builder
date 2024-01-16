// TODO: reuse with txpool/journal.go
package bundlepool

import (
	"errors"
	"io"
	"io/fs"
	"os"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/txpool"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
)

// devNull is a WriteCloser that just discards anything written into it. Its
// goal is to allow the bundle journal to write into a fake journal when
// loading bundles on startup without printing warnings due to no file
// being read for write.
type devNull struct{}

func (*devNull) Write(p []byte) (n int, err error) { return len(p), nil }
func (*devNull) Close() error                      { return nil }

type journal struct {
	path   string         // Filesystem path to store the bundles at
	writer io.WriteCloser // Output stream to write new bundles into
}

// newBundleJournal creates a new bundle journal
func newBundleJournal(path string) *journal {
	return &journal{
		path: path,
	}
}

// load parses a bundle journal dump from disk, loading its contents into
// the specified pool.
func (journal *journal) load(add func([]*types.Bundle) []error) error {
	// Open the journal for loading any past bundles
	input, err := os.Open(journal.path)
	if errors.Is(err, fs.ErrNotExist) {
		// Skip the parsing if the journal file doesn't exist at all
		return nil
	}
	if err != nil {
		return err
	}
	defer input.Close()

	// Temporarily discard any journal additions (don't double add on load)
	journal.writer = new(devNull)
	defer func() { journal.writer = nil }()

	// Inject all bundles from the journal into the pool
	stream := rlp.NewStream(input, 0)
	total, dropped := 0, 0

	// Create a method to load a limited batch of bundles and bump the
	// appropriate progress counters. Then use this method to load all the
	// journaled bundles in small-ish batches.
	loadBatch := func(bundles []*types.Bundle) {
		for _, err := range add(bundles) {
			if err != nil {
				log.Debug("Failed to add journaled bundle", "err", err)
				dropped++
			}
		}
	}
	var (
		failure error
		batch   []*types.Bundle
	)
	for {
		// Parse the next bundle and terminate on error
		bundle := new(types.Bundle)
		if err = stream.Decode(bundle); err != nil {
			if err != io.EOF {
				failure = err
			}
			if len(batch) > 0 {
				loadBatch(batch)
			}
			break
		}
		// New bundle parsed, queue up for later, import if threshold is reached
		total++

		if batch = append(batch, bundle); len(batch) > 1024 {
			loadBatch(batch)
			batch = batch[:0]
		}
	}
	log.Info("Loaded local bundle journal", "bundles", total, "dropped", dropped)

	return failure
}

// insert adds the specified bundle to the local disk journal.
func (journal *journal) insert(bundle *types.Bundle) error {
	if journal.writer == nil {
		return txpool.ErrNoActiveJournal
	}
	if err := rlp.Encode(journal.writer, bundle); err != nil {
		return err
	}
	return nil
}

// rotate regenerates the bundle journal based on the current contents of
// the bundle pool.
func (journal *journal) rotate(all map[common.Hash]*types.Bundle) error {
	// Close the current journal (if any is open)
	if journal.writer != nil {
		if err := journal.writer.Close(); err != nil {
			return err
		}
		journal.writer = nil
	}
	// Generate a new journal with the contents of the current pool
	replacement, err := os.OpenFile(journal.path+".new", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	for _, bundle := range all {
		if err = rlp.Encode(replacement, bundle); err != nil {
			replacement.Close()
			return err
		}
	}
	replacement.Close()

	// Replace the live journal with the newly generated one
	if err = os.Rename(journal.path+".new", journal.path); err != nil {
		return err
	}
	sink, err := os.OpenFile(journal.path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	journal.writer = sink
	log.Info("Regenerated local bundle journal", "bundles", len(all))

	return nil
}

// close flushes the bundle journal contents to disk and closes the file.
func (journal *journal) close() error {
	var err error

	if journal.writer != nil {
		err = journal.writer.Close()
		journal.writer = nil
	}
	return err
}
