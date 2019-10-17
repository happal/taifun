package main

// Filter decides whether to reject a Response.
type Filter interface {
	Reject(Response) bool
}
