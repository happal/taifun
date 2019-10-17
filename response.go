package main

import "time"

// Response is a response as received from a server.
type Response struct {
	Hide bool // can be set by a filter, response should not be displayed

	Item      string // requested hostname
	Addresses []string

	Duration time.Duration
	Error    error
}

// Mark runs the filters on all responses and marks those that should be hidden.
func Mark(in <-chan Response, filters []Filter) <-chan Response {
	ch := make(chan Response)

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
