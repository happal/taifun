package main

import "sort"

// Result is a response as received from a server.
type Result struct {
	Hide bool

	Item     string // requested item
	Hostname string // requested hostname

	Requests []Request
}

// Request contains the data for a request.
type Request struct {
	Hide bool // can be set by a filter, response should not be displayed

	Type     string // request type (A, AAAA, etc.)
	Status   string // dns response status (e.g. NXDOMAIN)
	Failure  bool   // set if status is anything else than NOERROR
	NotFound bool   // set if status is NXDOMAIN

	Error error

	Responses       []Response
	Nameserver, SOA []Response

	Raw struct {
		Question   []string
		Answer     []string
		Nameserver []string
		Extra      []string
	}
}

// Response contains the response to a DNS request.
type Response struct {
	Hide bool

	Type string
	Data string

	TTL uint
}

// Empty returns true if no responses returned any result (and no error was received either).
func (r Result) Empty() bool {
	for _, request := range r.Requests {
		if !request.Empty() {
			return false
		}
	}

	return true
}

// Delegation returns true if the responses indicate that this may be a degelated subdomain.
func (r Result) Delegation() bool {
	if !r.Empty() {
		return false
	}

	for _, request := range r.Requests {
		if len(request.Nameserver) > 0 || len(request.SOA) > 0 {
			return true
		}
	}

	return false
}

func unique(list []string) (cleaned []string) {
	known := make(map[string]struct{})
	for _, entry := range list {
		if _, ok := known[entry]; ok {
			continue
		}
		known[entry] = struct{}{}
		cleaned = append(cleaned, entry)
	}
	sort.Strings(cleaned)
	return cleaned
}

// Nameservers returns a list of (unique) name servers from SOA and NS records.
func (r Result) Nameservers() []string {
	var servers []string
	for _, req := range r.Requests {
		for _, res := range req.Nameserver {
			servers = append(servers, res.Data)
		}

		for _, res := range req.SOA {
			servers = append(servers, res.Data)
		}
	}
	return unique(servers)
}

// NewResponse returns a response.
func NewResponse(responseType string, ttl uint32, data string) Response {
	return Response{
		Type: responseType,
		TTL:  uint(ttl),
		Data: data,
	}
}

// Empty returns true if the response does not have any results and no error was returned.
func (r Request) Empty() bool {
	if r.Failure {
		return false
	}

	if len(r.Responses) > 0 {
		return false
	}

	return true
}

func runFilters(filters Filters, result Result) Result {
	for _, f := range filters.Result {
		if f.Reject(result) {
			result.Hide = true
			return result
		}
	}

	allRequestsHidden := true
	for i, request := range result.Requests {
		requestHidden := false
		for _, requestFilter := range filters.Request {
			if requestFilter.Reject(request) {
				requestHidden = true
				result.Requests[i].Hide = true
				break // continue to next request
			}

			for j, response := range request.Responses {
				for _, responseFilter := range filters.Response {
					if responseFilter.Reject(response) {
						request.Responses[j].Hide = true
						break // continue to next response
					}
				}
			}
		}

		if !requestHidden {
			allRequestsHidden = false
		}
	}

	// mark the whole result as hidden there are no requests
	if allRequestsHidden {
		result.Hide = true
	}

	return result
}

// Mark runs the filters on all results and marks those that should be hidden.
func Mark(in <-chan Result, filters Filters) <-chan Result {
	ch := make(chan Result)

	go func() {
		defer close(ch)
		for res := range in {
			res = runFilters(filters, res)
			ch <- res
		}
	}()

	return ch
}
