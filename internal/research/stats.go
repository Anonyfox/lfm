package research

import (
	"sort"
	"strings"
	"time"
)

// Count is a value with its frequency.
type Count struct {
	Name string
	N    int
}

// Summary describes the research a page stands on.
type Summary struct {
	Voices, Peers, Batches int
	Platforms              []Count // by voice count, largest first
	Dated                  int     // voices with a posted date
	Oldest, Newest         time.Time
	Recent                 int // voices posted within a year of the latest fetch
	Latest                 time.Time
}

// Empty reports whether there is no research to describe.
func (s Summary) Empty() bool { return s.Voices == 0 && s.Peers == 0 }

func summarize(v *Voices, c *Competitors) Summary {
	var s Summary
	var fetched time.Time
	if v != nil {
		s.Voices, s.Batches = len(v.Entries), len(v.Batches)
		platforms := map[string]int{}
		names := map[string]string{}
		for _, e := range v.Entries {
			key := strings.ToLower(e.Platform)
			platforms[key]++
			if names[key] == "" {
				names[key] = e.Platform
			}
			if e.Fetched.After(fetched) {
				fetched = e.Fetched
			}
			if e.Posted.IsZero() {
				continue
			}
			s.Dated++
			if s.Oldest.IsZero() || e.Posted.Before(s.Oldest) {
				s.Oldest = e.Posted
			}
			if e.Posted.After(s.Newest) {
				s.Newest = e.Posted
			}
		}
		for key, n := range platforms {
			s.Platforms = append(s.Platforms, Count{names[key], n})
		}
		sort.Slice(s.Platforms, func(i, j int) bool {
			if s.Platforms[i].N != s.Platforms[j].N {
				return s.Platforms[i].N > s.Platforms[j].N
			}
			return s.Platforms[i].Name < s.Platforms[j].Name
		})
		for _, e := range v.Entries {
			if !e.Posted.IsZero() && !e.Posted.Before(fetched.AddDate(-1, 0, 0)) {
				s.Recent++
			}
		}
		for _, b := range v.Batches {
			if b.Date.After(s.Latest) {
				s.Latest = b.Date
			}
		}
	}
	if c != nil {
		s.Peers = len(c.Entries)
		s.Batches += len(c.Batches)
		for _, b := range c.Batches {
			if b.Date.After(s.Latest) {
				s.Latest = b.Date
			}
		}
	}
	return s
}
