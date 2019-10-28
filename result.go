package main

import (
	"fmt"
	"sort"
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
		return fmt.Sprintf("%s not found", r.Hostname)
	}

	if r.Empty() {
		return fmt.Sprintf("empty response, subdomain?")
	}

	addrs := r.Addresses()
	names := r.CNAMEs()
	sort.Strings(addrs)
	sort.Strings(names)

	if len(addrs) > 0 && len(names) == 0 {
		return strings.Join(addrs, "  ")
	}

	if len(addrs) == 0 && len(names) > 0 {
		return "CNAME " + strings.Join(names, ", ")
	}

	return fmt.Sprintf("%s (CNAME %s)", strings.Join(addrs, "  "), strings.Join(names, ", "))
}

// CNAMEs returns a list of CNAME responses. Duplicate names are removed.
func (r Result) CNAMEs() (list []string) {
	names := make(map[string]struct{})
	for _, name := range r.A.CNAMEs {
		names[name] = struct{}{}
	}
	for _, name := range r.AAAA.CNAMEs {
		names[name] = struct{}{}
	}

	for name := range names {
		list = append(list, name)
	}

	return list
}

// Addresses returns a list of Addresses. Duplicate addresse are removed.
func (r Result) Addresses() (list []string) {
	addrs := make(map[string]struct{})
	for _, addr := range r.A.Addresses {
		addrs[addr] = struct{}{}
	}
	for _, addr := range r.AAAA.Addresses {
		addrs[addr] = struct{}{}
	}

	for addr := range addrs {
		list = append(list, addr)
	}

	return list
}

// Empty returns true if all responses are empty.
func (r Result) Empty() bool {
	if !r.A.Empty() {
		return false
	}
	if !r.AAAA.Empty() {
		return false
	}
	return true
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
