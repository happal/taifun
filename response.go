package main

import (
	"fmt"
	"net"
	"strings"
	"time"
)

// Response is a response as received from a server.
type Response struct {
	Hide bool // can be set by a filter, response should not be displayed

	Item      string // requested item and hostname
	Hostname  string
	Addresses []string

	Duration time.Duration
	Error    error
}

// IsNotFound returns true if the requested hostname could not be found.
func (r Response) IsNotFound() bool {
	if r.Error == nil {
		return false
	}

	err, ok := r.Error.(*net.DNSError)
	if !ok {
		return false
	}

	return err.IsNotFound
}

func (r Response) String() string {
	if r.IsNotFound() {
		return fmt.Sprintf("%-30s %-16s", r.Hostname, "error: not found")
	}

	return fmt.Sprintf("%-30s %-16s", r.Hostname, strings.Join(r.Addresses, ", "))
}

// Mark runs the filters on all responses and marks those that should be hidden.
func Mark(in <-chan Response, filters []Filter) <-chan Response {
	ch := make(chan Response)

	go func() {
		defer close(ch)
		for res := range in {
			// run filters
			hide := false
			for _, f := range filters {
				if f.Reject(res) {
					hide = true
					break
				}
			}
			res.Hide = hide

			// forward response to next in chain
			ch <- res
		}
	}()

	return ch
}
