package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/happal/taifun/cli"
)

// Reporter prints the Responses to a terminal.
type Reporter struct {
	term cli.Terminal
}

// NewReporter returns a new reporter.
func NewReporter(term cli.Terminal) *Reporter {
	return &Reporter{term: term}
}

// Stats collects statistics about several responses.
type Stats struct {
	Start      time.Time
	Errors     int
	Responses  int
	IPv4, IPv6 map[string]struct{}

	ShownResponses int
	Count          int

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
	status := fmt.Sprintf("%v of %v requests shown", h.ShownResponses, h.Responses)
	dur := time.Since(h.Start) / time.Second

	if dur > 0 && time.Since(h.lastRPS) > time.Second {
		h.rps = float64(h.Responses) / float64(dur)
		h.lastRPS = time.Now()
	}

	if h.rps > 0 {
		status += fmt.Sprintf(", %.0f req/s", h.rps)
	}

	todo := h.Count - h.Responses
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
	res = append(res, fmt.Sprintf("IPv4:      %v", len(h.IPv4)))
	res = append(res, fmt.Sprintf("IPv6:      %v", len(h.IPv6)))

	return res
}

// Display shows incoming Responses.
func (r *Reporter) Display(ch <-chan Response, countChannel <-chan int) error {
	r.term.Printf("%-30s %-16s\n", "name", "response")

	stats := &Stats{
		Start: time.Now(),
		IPv4:  make(map[string]struct{}),
		IPv6:  make(map[string]struct{}),
	}

	for response := range ch {
		select {
		case c := <-countChannel:
			stats.Count = c
		default:
		}

		stats.Responses++
		if response.Error != nil {
			stats.Errors++
		} else {
			for _, addr := range response.Addresses {
				if strings.Contains(addr, ":") {
					stats.IPv6[addr] = struct{}{}
				} else {
					stats.IPv4[addr] = struct{}{}
				}
			}
		}

		if !response.Hide {
			r.term.Printf("%v\n", response)
			stats.ShownResponses++
		}

		r.term.SetStatus(stats.Report(response.Item))
	}

	r.term.Print("\n")
	r.term.Printf("processed %d HTTP requests in %v\n", stats.Responses, formatSeconds(time.Since(stats.Start).Seconds()))

	for _, line := range stats.Report("")[1:] {
		r.term.Print(line)
	}

	return nil
}
