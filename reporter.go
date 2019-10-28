package main

import (
	"fmt"
	"time"

	"github.com/happal/taifun/cli"
)

// Reporter prints the Results to a terminal.
type Reporter struct {
	term cli.Terminal
}

// NewReporter returns a new reporter.
func NewReporter(term cli.Terminal) *Reporter {
	return &Reporter{term: term}
}

// Stats collects statistics about several responses.
type Stats struct {
	Start          time.Time
	Errors         int
	Results        int
	A, AAAA, CNAME map[string]struct{}

	ShownResults int
	Count        int

	lastRPS time.Time
	rps     float64
}

func formatSeconds(secs float64) string {
	sec := int(secs)
	hours := sec / 3600
	sec -= hours * 3600
	min := sec / 60
	sec -= min * 60

	if hours > 0 {
		return fmt.Sprintf("%dh%02dm%02ds", hours, min, sec)
	}

	return fmt.Sprintf("%dm%02ds", min, sec)
}

// Report returns a report about the received HTTP status codes.
func (h *Stats) Report(current string) (res []string) {
	res = append(res, "")
	status := fmt.Sprintf("%v of %v requests shown", h.ShownResults, h.Results)
	dur := time.Since(h.Start) / time.Second

	if dur > 0 && time.Since(h.lastRPS) > time.Second {
		h.rps = float64(h.Results) / float64(dur)
		h.lastRPS = time.Now()
	}

	if h.rps > 0 {
		status += fmt.Sprintf(", %.0f req/s", h.rps)
	}

	todo := h.Count - h.Results
	if todo > 0 {
		status += fmt.Sprintf(", %d todo", todo)

		if h.rps > 0 {
			rem := float64(todo) / h.rps
			status += fmt.Sprintf(", %s remaining", formatSeconds(rem))
		}
	}

	if current != "" {
		status += fmt.Sprintf(", current: %v", current)
	}

	res = append(res, status)

	res = append(res, fmt.Sprintf("errors:    %v", h.Errors))
	res = append(res, fmt.Sprintf("A:         %v", len(h.A)))
	res = append(res, fmt.Sprintf("AAAA:      %v", len(h.AAAA)))
	res = append(res, fmt.Sprintf("CNAME:     %v", len(h.CNAME)))

	return res
}

// Display shows incoming Results.
func (r *Reporter) Display(ch <-chan Result, countChannel <-chan int) error {
	r.term.Printf("%16s   %-16s", "name", "result")

	stats := &Stats{
		Start: time.Now(),
		A:     make(map[string]struct{}),
		AAAA:  make(map[string]struct{}),
		CNAME: make(map[string]struct{}),
	}

	for result := range ch {
		select {
		case c := <-countChannel:
			stats.Count = c
		default:
		}

		stats.Results++

		if result.A.Error != nil {
			stats.Errors++
		} else {
			for _, addr := range result.A.Addresses {
				stats.A[addr] = struct{}{}
			}
			for _, name := range result.A.CNAMEs {
				stats.CNAME[name] = struct{}{}
			}
		}

		if result.AAAA.Error != nil {
			stats.Errors++
		} else {
			for _, addr := range result.AAAA.Addresses {
				stats.AAAA[addr] = struct{}{}
			}
			for _, name := range result.AAAA.CNAMEs {
				stats.CNAME[name] = struct{}{}
			}
		}

		if !result.Hide {
			r.term.Printf("%16s  %s", result.Hostname, result)
			stats.ShownResults++
		}

		r.term.SetStatus(stats.Report(result.Item))
	}

	r.term.Print("\n")
	r.term.Printf("processed %d HTTP requests in %v\n", stats.Results, formatSeconds(time.Since(stats.Start).Seconds()))

	for _, line := range stats.Report("")[1:] {
		r.term.Print(line)
	}

	return nil
}
