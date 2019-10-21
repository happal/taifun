package main

// Filter decides whether to reject a Response.
type Filter interface {
	Reject(Response) bool
}

// FilterFunc wraps a function so that it implements thi Filter interface.
type FilterFunc func(Response) bool

// Reject runs f on the Response.
func (f FilterFunc) Reject(r Response) bool {
	return f(r)
}

// FilterNotFound returns a filter which hides "not found" responses.
func FilterNotFound() Filter {
	return FilterFunc(func(r Response) bool {
		return r.Status == "NXDOMAIN"
	})
}
