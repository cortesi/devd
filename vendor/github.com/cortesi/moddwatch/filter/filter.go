package filter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar"
)

func matchAny(path string, patterns []string) (bool, error) {
	for _, pattern := range patterns {
		match, err := doublestar.Match(pattern, filepath.ToSlash(path))
		if err != nil {
			return false, fmt.Errorf("Error matching pattern '%s': %s", pattern, err)
		} else if match {
			return true, nil
		}
	}
	return false, nil
}

// Determine if a file should be included, based on the given exclude paths.
func shouldInclude(file string, includePatterns []string, excludePatterns []string) (bool, error) {
	include, err := matchAny(file, includePatterns)
	if err != nil || include == false {
		return false, err
	}

	exclude, err := matchAny(file, excludePatterns)
	if err != nil {
		return false, err
	}
	if exclude == true {
		return false, err
	}
	return true, nil
}

// Files filters an array of files. At least ONE include pattern must match,
// and NONE of the exclude patterns must match.
func Files(files []string, includePatterns []string, excludePatterns []string) ([]string, error) {
	ret := []string{}
	for _, file := range files {
		ok, err := shouldInclude(file, includePatterns, excludePatterns)
		if err != nil {
			return files, err
		}
		if ok {
			ret = append(ret, file)
		}
	}
	return ret, nil
}

// BaseDir returns the base directory for a match pattern
func BaseDir(pattern string) string {
	split := strings.IndexAny(pattern, "*{}?[]")
	if split >= 0 {
		pattern = pattern[:split]
	}
	dir := filepath.Dir(pattern)
	return filepath.Clean(dir)
}

// isUnder checks if directory b is under directory a. Note that arguments MUST
// be directory specifications, not files.
func isUnder(a, b string) bool {
	a, err := filepath.Abs(a)
	if err != nil {
		return false
	}
	b, err = filepath.Abs(b)
	if err != nil {
		return false
	}
	if strings.HasPrefix(b, a) {
		return true
	}
	return false
}

// AppendBaseDirs traverses a slice of patterns, and appends them to the
// bases list. The result of the append operation is a minimal set - that
// is, paths that are redundant because an enclosing path is already included
// are removed.
func AppendBaseDirs(bases []string, patterns []string) []string {
Loop:
	for _, s := range patterns {
		s = BaseDir(s)
		for i, e := range bases {
			if isUnder(s, e) {
				bases[i] = s
				continue Loop
			} else if isUnder(e, s) {
				continue Loop
			}
		}
		bases = append(bases, s)
	}
	return bases
}

// Find all files under the root that match the specified patterns. All
// arguments and returned paths are slash-delimited.
func Find(root string, includePatterns []string, excludePatterns []string) ([]string, error) {
	root = filepath.FromSlash(root)
	bases := AppendBaseDirs([]string{}, includePatterns)
	ret := []string{}
	for _, b := range bases {
		b = filepath.FromSlash(b)
		err := filepath.Walk(filepath.Join(root, b), func(p string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			cleanpath, err := filepath.Rel(root, p)
			if err != nil {
				return nil
			}
			cleanpath = filepath.ToSlash(cleanpath)

			excluded, err := matchAny(cleanpath, excludePatterns)
			if err != nil {
				return nil
			}
			if fi.IsDir() {
				if excluded {
					return filepath.SkipDir
				}
			} else if !excluded {
				included, err := matchAny(cleanpath, includePatterns)
				if err != nil {
					return nil
				}
				if included {
					ret = append(ret, cleanpath)
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return ret, nil
}
