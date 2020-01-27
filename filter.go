package main

import (
	"net"
	"regexp"
)

// RequestFilter decides whether to reject a Request/Response.
type RequestFilter interface {
	Reject(Request) bool
}

// RequestFilterFunc wraps a function so that it implements thi Filter interface.
type RequestFilterFunc func(Request) bool

// Reject runs f on the Request.
func (f RequestFilterFunc) Reject(r Request) bool {
	return f(r)
}

// ResultFilter decides whether to reject a Result.
type ResultFilter interface {
	Reject(Result) bool
}

// ResultFilterFunc wraps a function so that it implements thi Filter interface.
type ResultFilterFunc func(Result) bool

// Reject runs f on the Result.
func (f ResultFilterFunc) Reject(r Result) bool {
	return f(r)
}

// ResponseFilter decides whether to reject a Response.
type ResponseFilter interface {
	Reject(Response) bool
}

// ResponseFilterFunc wraps a function so that it implements thi Filter interface.
type ResponseFilterFunc func(Response) bool

// Reject runs f on the Response.
func (f ResponseFilterFunc) Reject(r Response) bool {
	return f(r)
}

// FilterNotFound returns a filter which hides "not found" responses.
func FilterNotFound() RequestFilter {
	return RequestFilterFunc(func(r Request) (reject bool) {
		return r.NotFound
	})
}

// FilterInSubnet returns a filter which hides responses with addresses in one
// of the subnets.
func FilterInSubnet(subnets []*net.IPNet) ResponseFilter {
	return ResponseFilterFunc(func(res Response) (reject bool) {
		// don't process anything except v4/v6 responses
		if res.Type != "A" && res.Type != "AAAA" {
			return false
		}

		ip := net.ParseIP(res.Data)
		if ip == nil {
			return false
		}

		for _, subnet := range subnets {
			if subnet.Contains(ip) {
				return true
			}
		}

		return false
	})
}

// FilterNotInSubnet returns a filter which hides responses with addresses
// which are not in one of the subnets.
func FilterNotInSubnet(subnets []*net.IPNet) ResponseFilter {
	return ResponseFilterFunc(func(res Response) (reject bool) {
		// don't process anything except v4/v6 responses
		if res.Type != "A" && res.Type != "AAAA" {
			return false
		}

		ip := net.ParseIP(res.Data)
		if ip == nil {
			return false
		}

		for _, subnet := range subnets {
			if subnet.Contains(ip) {
				return false
			}
		}

		return true
	})
}

// FilterEmptyResults returns a filter which hides responses.
func FilterEmptyResults() ResultFilter {
	return ResultFilterFunc(func(r Result) (reject bool) {
		return r.Empty()
	})
}

// FilterDelegations returns a filter which hides potential delegations.
func FilterDelegations() ResultFilter {
	return ResultFilterFunc(func(r Result) (reject bool) {
		return r.Delegation()
	})
}

// FilterRejectCNAMEs return a filter which hides cnames matching any of the patterns.
func FilterRejectCNAMEs(patterns []*regexp.Regexp) ResponseFilter {
	return ResponseFilterFunc(func(r Response) (reject bool) {
		if r.Type != "CNAME" {
			return false
		}

		for _, pat := range patterns {
			if pat.Match([]byte(r.Data)) {
				return true
			}
		}

		return false
	})
}

// FilterRejectPTR returns a filter which hides PTR responses matching one of the patterns.
func FilterRejectPTR(patterns []*regexp.Regexp) ResponseFilter {
	return ResponseFilterFunc(func(r Response) (reject bool) {
		if r.Type != "PTR" {
			return false
		}

		for _, pat := range patterns {
			if pat.Match([]byte(r.Data)) {
				return true
			}
		}

		return false
	})
}
