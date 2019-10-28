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

	Nameservers []string // set to the list of potential nameservers for delegated subdomains
}

func (r Result) String() (result string) {
	if r.NotFound {
		return fmt.Sprintf("%s not found", r.Hostname)
	}

	if r.Delegation() {
		return fmt.Sprintf("delegation, servers: %s", strings.Join(r.Nameservers, ", "))
	}

	if r.Empty() {
		return "empty response, potential suffix"
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

// unique returns a sorted copy of list with all duplicates removed.
func unique(list []string) []string {
	res := make(map[string]struct{})
	for _, entry := range list {
		res[entry] = struct{}{}
	}
	list2 := make([]string, 0, len(res))
	for entry := range res {
		list2 = append(list2, entry)
	}
	sort.Strings(list2)
	return list2
}

// CNAMEs returns a list of CNAME responses. Duplicate names are removed.
func (r Result) CNAMEs() (list []string) {
	list = append(list, r.A.CNAMEs...)
	list = append(list, r.AAAA.CNAMEs...)
	return unique(list)
}

// Addresses returns a list of Addresses. Duplicate addresse are removed.
func (r Result) Addresses() (list []string) {
	list = append(list, r.A.Addresses...)
	list = append(list, r.AAAA.Addresses...)
	return unique(list)
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

// Delegation returns true if the responses indicate that this may be a degelated subdomain.
func (r *Result) Delegation() bool {
	if !r.A.Empty() || !r.AAAA.Empty() {
		return false
	}

	if r.Nameservers == nil {
		var list []string
		list = append(list, r.A.Nameserver...)
		list = append(list, r.A.SOA...)
		list = append(list, r.AAAA.Nameserver...)
		list = append(list, r.AAAA.SOA...)
		r.Nameservers = unique(list)
	}

	return len(r.Nameservers) > 0
}

// Response is a response for a specific DNS request.
type Response struct {
	Status  string // dns response status (e.g. NXDOMAIN)
	Failure bool   // set if status is anything else than NOERROR

	Addresses  []string
	CNAMEs     []string
	SOA        []string
	Nameserver []string
	Error      error
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
