package main

import "net"

// Filter decides whether to reject a Result.
type Filter interface {
	Reject(Result) bool
}

// FilterFunc wraps a function so that it implements thi Filter interface.
type FilterFunc func(Result) bool

// Reject runs f on the Result.
func (f FilterFunc) Reject(r Result) bool {
	return f(r)
}

// FilterNotFound returns a filter which hides "not found" responses.
func FilterNotFound() Filter {
	return FilterFunc(func(r Result) (reject bool) {
		return r.NotFound
	})
}

// FilterInSubnet returns a filter which hides responses with addresses in one
// of the subnets.
func FilterInSubnet(subnets []*net.IPNet) Filter {
	return FilterFunc(func(r Result) (reject bool) {
		for _, addr := range r.Addresses() {
			ip := net.ParseIP(addr)
			if ip == nil {
				continue
			}

			for _, subnet := range subnets {
				if subnet.Contains(ip) {
					return true
				}
			}
		}

		return false
	})
}

// FilterNotInSubnet returns a filter which hides responses with addresses
// which are not in one of the subnets.
func FilterNotInSubnet(subnets []*net.IPNet) Filter {
	return FilterFunc(func(r Result) (reject bool) {
		for _, addr := range r.Addresses() {
			ip := net.ParseIP(addr)
			if ip == nil {
				continue
			}

			for _, subnet := range subnets {
				if subnet.Contains(ip) {
					return false
				}
			}
		}

		return true
	})
}
