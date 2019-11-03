package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/happal/taifun/cli"
)

// Reporter prints the Results to a terminal.
type Reporter struct {
	term  cli.Terminal
	width int
}

// NewReporter returns a new reporter, width is the length of the hostname
// template (used for the first column).
func NewReporter(term cli.Terminal, width int) *Reporter {
	return &Reporter{term: term, width: width}
}

// Stats collects statistics about several responses.
type Stats struct {
	Start            time.Time
	Errors, Results  int
	Empty, Delegated int
	A, AAAA, CNAME   map[string]struct{}

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

// Report returns a report about the received response codes.
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

	if h.Errors > 0 {
		res = append(res, fmt.Sprintf("errors:       %v", h.Errors))
	}
	if len(h.A) > 0 {
		res = append(res, fmt.Sprintf("unique A:     %v", len(h.A)))
	}
	if len(h.AAAA) > 0 {
		res = append(res, fmt.Sprintf("unique AAAA:  %v", len(h.AAAA)))
	}
	if len(h.CNAME) > 0 {
		res = append(res, fmt.Sprintf("unique CNAME: %v", len(h.CNAME)))
	}
	if h.Empty > 0 {
		res = append(res, fmt.Sprintf("empty:        %v", h.Empty))
	}
	if h.Delegated > 0 {
		res = append(res, fmt.Sprintf("delegated:    %v", h.Delegated))
	}

	return res
}

func ljust(s string, width int) string {
	if len(s) < width {
		return strings.Repeat(" ", width-len(s)) + s
	}
	return s
}

type printer interface {
	Printf(string, ...interface{})
}

func printResponse(term printer, width int, result Result) {
	if result.Delegation() {
		var servers []string
		for _, res := range result.Nameserver {
			servers = append(servers, res.Data)
		}

		term.Printf("%s   potential delegation, servers: %s",
			ljust(result.Hostname, width),
			strings.Join(servers, ", "))
	}

	if result.Empty() {
		term.Printf("%s   empty response, potential suffix\n", ljust(result.Hostname, width))
	}

	for _, response := range result.Responses {
		term.Printf("%s   %v %v %v %v\n",
			ljust(result.Hostname, width),
			result.RequestType,
			response.Type,
			response.TTL,
			response.Data,
		)
	}
}

// Display shows incoming Results.
func (r *Reporter) Display(ch <-chan Result, countChannel <-chan int) error {
	r.term.Printf("%s     result", ljust("name  ", r.width))

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

		if result.Delegation() {
			stats.Delegated++
		} else if result.Empty() {
			stats.Empty++
		}

		if result.Error != nil {
			stats.Errors++
		}

		for _, response := range result.Responses {
			switch response.Type {
			case "A":
				stats.A[response.Data] = struct{}{}
			case "AAAA":
				stats.AAAA[response.Data] = struct{}{}
			case "CNAME":
				stats.CNAME[response.Data] = struct{}{}
			}
		}

		if !result.Hide {
			printResponse(r.term, r.width, result)
			stats.ShownResults++
		}

		r.term.SetStatus(stats.Report(result.Item))
	}

	r.term.Print("\n")
	r.term.Printf("resolved %d DNS requests in %v\n", stats.Results, formatSeconds(time.Since(stats.Start).Seconds()))

	for _, line := range stats.Report("")[1:] {
		r.term.Print(line)
	}

	return nil
}
