package main

// Result is a response as received from a server.
type Result struct {
	Hide bool // can be set by a filter, response should not be displayed

	Item        string // requested item
	Hostname    string // requested hostname
	RequestType string // request type (A, AAAA, etc.)

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
	Type string
	Data string

	TTL uint
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

	return len(r.Nameserver) > 0 || len(r.SOA) > 0
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
