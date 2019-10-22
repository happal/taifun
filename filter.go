package main

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
	return FilterFunc(func(r Result) bool {
		return r.Status == "NXDOMAIN"
	})
}
