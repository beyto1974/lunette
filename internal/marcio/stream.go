package marcio

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// ErrStop ends a walk early. A caller that has seen all it needs - `show -n 1`
// against a harvest of a million records - returns it from the batch callback,
// and the walk stops without reading the rest of the file and without calling
// the stop a failure.
var ErrStop = errors.New("marcio: stop walking")

// Summary is everything a Result knows except the records themselves: what a
// streaming read can report once it has finished, without ever having held the
// whole file in memory.
type Summary struct {
	Format Format
	// MixedFormats reports that the inputs were not all the same format.
	MixedFormats bool
	ForcedUTF8   bool
	// Records counts the records that decoded, Issues lists the ones that did
	// not. A walk that stopped early counts only what it reached.
	Records int
	Issues  []Issue
	// Stopped reports that the callback asked for the walk to end, so the
	// counts describe a prefix of the input rather than all of it.
	Stopped bool
}

// StreamReader reads r as one input, handing batchSize records at a time to
// fn. It is Load without the records piling up.
func StreamReader(r io.Reader, batchSize int, fn func(Batch) error) (*Summary, error) {
	sum := &Summary{}
	res, err := walk(r, batchSize, walkOpts{}, sum.collect(fn))
	if err != nil {
		return nil, err
	}
	sum.absorb(res, true)
	return sum, nil
}

// StreamFiles reads several files as one set, handing batchSize records at a
// time to fn. It is LoadFiles without the records piling up: record numbering
// and issue sources are identical, so an issue reported against "record 402"
// means the same thing either way.
func StreamFiles(paths []string, batchSize int, fn func(Batch) error) (*Summary, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("no files given")
	}

	sum := &Summary{}
	for i, path := range paths {
		// A failure from the callback belongs to the caller, not to the file:
		// naming the file in front of it would blame the wrong thing.
		var fromCallback error
		cb := func(b Batch) error {
			err := sum.collect(fn)(b)
			fromCallback = err
			return err
		}
		res, err := streamOne(path, batchSize, walkOpts{base: sum.Records, source: path}, cb)
		if err != nil {
			if fromCallback != nil {
				return nil, fromCallback
			}
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		sum.absorb(res, i == 0)
		if sum.Stopped {
			break
		}
	}
	return sum, nil
}

// streamOne opens a file and walks it. The file is closed before the next one
// is opened, so a set of a thousand harvest files needs one descriptor.
func streamOne(path string, batchSize int, opts walkOpts, fn func(Batch) error) (walkResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return walkResult{}, err
	}
	defer f.Close()
	return walk(f, batchSize, opts, fn)
}

// collect wraps the caller's callback so that issues reach the summary as well
// as the caller. Issues are kept whole because `validate` lists them; the
// records they replace are not kept at all.
func (s *Summary) collect(fn func(Batch) error) func(Batch) error {
	return func(b Batch) error {
		s.Issues = append(s.Issues, b.Issues...)
		return fn(b)
	}
}

// absorb folds one input's outcome into the summary. first says whether this
// input set the format or has to agree with it.
func (s *Summary) absorb(res walkResult, first bool) {
	switch {
	case first:
		s.Format = res.format
	case res.format != s.Format:
		s.MixedFormats = true
	}
	s.ForcedUTF8 = s.ForcedUTF8 || res.forcedUTF8
	s.Records += res.decoded
	s.Stopped = s.Stopped || res.stopped
}

// DescribeSummaryFormat names the format of a streamed read, allowing for a
// mixed set, as DescribeFormat does for a Result.
func DescribeSummaryFormat(s *Summary) string {
	if s.MixedFormats {
		return s.Format.String() + " (mixed)"
	}
	return s.Format.String()
}
