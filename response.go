package main

import (
	"fmt"
	"strings"
	"time"
)

// Response is a response as received from a server.
type Response struct {
	Hide bool // can be set by a filter, response should not be displayed

	Item     string // requested item and hostname
	Hostname string

	Status  string // dns response status (e.g. NXDOMAIN)
	Failure bool   // set if status is anything else than NOERROR

	Addresses []string
	CNAMES    []string

	Duration time.Duration
	Error    error
}

func (r Response) String() string {
	if r.Failure {
		return fmt.Sprintf("%-30s lookup failure: %-16s", r.Hostname, r.Status)
	}

	if r.Error != nil {
		return fmt.Sprintf("%-30s error: %v", r.Hostname, r.Error)
	}

	addrs := ""
	if len(r.Addresses) > 0 {
		addrs += strings.Join(r.Addresses, ", ")
	}

	if len(r.CNAMES) > 0 {
		addrs += fmt.Sprintf("CNAME %v", strings.Join(r.CNAMES, ", "))
	}

	return fmt.Sprintf("%-30s %-16s", r.Hostname, addrs)
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
