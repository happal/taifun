package main

// Result is a response as received from a server.
type Result struct {
	Hide     bool   // can be set by a filter, response should not be displayed
	Item     string // requested item
	Hostname string // requested hostname
	Type     string // request type (A, AAAA, etc.)

	Error error

	DNSResponse
}

// DNSResponse contains the response to a DNS request.
type DNSResponse struct {
	Status   string // dns response status (e.g. NXDOMAIN)
	Failure  bool   // set if status is anything else than NOERROR
	NotFound bool   // set if status is NXDOMAIN

	Responses       []string
	CNAMEs          []string
	Nameserver, SOA []string
	TTL             uint

	Raw struct {
		Question   []string
		Answer     []string
		Nameserver []string
		Extra      []string
	}
}

// Empty returns true if the response does not have any results and no error was returned.
func (r Result) Empty() bool {
	if r.Failure {
		return false
	}

	if len(r.Responses) > 0 {
		return false
	}

	return true
}

// Delegation returns true if the responses indicate that this may be a degelated subdomain.
func (r *Result) Delegation() bool {
	if !r.Empty() {
		return false
	}

	return len(r.Nameserver) > 0
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
