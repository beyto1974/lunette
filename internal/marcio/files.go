package marcio

import "fmt"

// LoadFiles reads several files as one set of records.
//
// A harvest arrives in pieces - metha writes a file per request window - and
// concatenating them first only works for the binary format: two MARCXML
// documents cannot be joined end to end. Reading them here sidesteps that, and
// keeps the record numbering running across the whole set so that an issue
// reported against "record 402" means something.
func LoadFiles(paths []string) (*Result, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("no files given")
	}

	combined := &Result{}
	for i, path := range paths {
		res, err := LoadFile(path)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}

		switch {
		case i == 0:
			combined.Format = res.Format
		case res.Format != combined.Format:
			combined.MixedFormats = true
		}
		combined.ForcedUTF8 = combined.ForcedUTF8 || res.ForcedUTF8

		// Ordinals and offsets are per-file; carry the running record count so
		// they read as positions in the whole set, and name the file each
		// issue came from.
		before := len(combined.Records)
		for _, issue := range res.Issues {
			issue.Ordinal += before
			issue.Source = path
			combined.Issues = append(combined.Issues, issue)
		}

		combined.Records = append(combined.Records, res.Records...)
		combined.Offsets = append(combined.Offsets, res.Offsets...)
	}
	return combined, nil
}

// Describe names a set of inputs for a report: the file itself when there is
// one, otherwise the first and a count.
func Describe(paths []string) string {
	switch len(paths) {
	case 0:
		return "nothing"
	case 1:
		return paths[0]
	default:
		return fmt.Sprintf("%s and %d more", paths[0], len(paths)-1)
	}
}

// DescribeFormat names the format of a result, allowing for a mixed set.
func DescribeFormat(res *Result) string {
	if res.MixedFormats {
		return res.Format.String() + " (mixed)"
	}
	return res.Format.String()
}
