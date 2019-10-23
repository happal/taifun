package main

import (
	"fmt"
	"strings"
)

// Result is a response as received from a server.
type Result struct {
	Hide     bool // can be set by a filter, response should not be displayed
	NotFound bool // set to true if all responses are NXDOMAIN

	Item     string // requested item and hostname
	Hostname string

	A    Response
	AAAA Response
}

func (r Result) String() (result string) {
	if r.NotFound {
		return fmt.Sprintf("%-30s %-16s", r.Hostname, "not found")
	}

	return fmt.Sprintf("%-30s %-16s %-16s", r.Hostname, r.A, r.AAAA)
}

// Response is a response for a specific DNS request.
type Response struct {
	Status  string // dns response status (e.g. NXDOMAIN)
	Failure bool   // set if status is anything else than NOERROR

	Addresses []string
	CNAMEs    []string
	Error     error
}

func (r Response) String() string {
	if r.Failure {
		return fmt.Sprintf("%v (%v)", r.Status, r.Error)
	}

	var fields []string
	if len(r.Addresses) > 0 {
		fields = append(fields, strings.Join(r.Addresses, ", "))
	}
	if len(r.CNAMEs) > 0 {
		fields = append(fields, fmt.Sprintf("CNAME %v", strings.Join(r.CNAMEs, ", ")))
	}

	return strings.Join(fields, "  ")
}

// Empty returns true if the response does not have any results and no error was returned.
func (r Response) Empty() bool {
	if r.Failure {
		return false
	}

	if len(r.Addresses) > 0 {
		return false
	}

	if len(r.CNAMEs) > 0 {
		return false
	}

	return true
}

// Mark runs the filters on all responses and marks those that should be hidden.
func Mark(in <-chan Result, filters []Filter) <-chan Result {
	ch := make(chan Result)

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
