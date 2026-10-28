package main

import (
	"fmt"
	"strconv"
	"strings"
)

type artifact struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Expired bool   `json:"expired"`
}

// Stable names survive partial retries. Legacy names are ordered by attempt,
// never by API listing order; duplicate results for an attempt fail closed.
func selectArtifacts(all []artifact, expected []string, run string, attempt int) ([]string, error) {
	ids := []string{}
	for _, key := range expected {
		prefix := "oci-" + key + "-" + run
		chosen := artifact{}
		rank := -1
		seen := map[int]bool{}
		for _, a := range all {
			r := attempt + 1
			if a.Name != prefix {
				suffix, ok := strings.CutPrefix(a.Name, prefix+"-")
				if !ok {
					continue
				}
				n, err := strconv.Atoi(suffix)
				if err != nil || n < 1 || n > attempt {
					return nil, fmt.Errorf("invalid artifact attempt: %s", a.Name)
				}
				r = n
			}
			if seen[r] {
				return nil, fmt.Errorf("ambiguous artifact for %s", key)
			}
			seen[r] = true
			if r > rank {
				chosen, rank = a, r
			}
		}
		if rank < 0 || chosen.Expired || chosen.ID <= 0 {
			return nil, fmt.Errorf("missing or expired artifact for %s", key)
		}
		ids = append(ids, strconv.FormatInt(chosen.ID, 10))
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no required build artifacts")
	}
	return ids, nil
}
