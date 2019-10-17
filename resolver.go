package main

import (
	"context"
	"net"
	"strings"
	"time"
)

// Resolver executes DNS requests.
type Resolver struct {
	input  <-chan string
	output chan<- Response

	template string
}

// NewResolver returns a new resolver with the given input and output channels.
func NewResolver(in <-chan string, out chan<- Response, template string) *Resolver {
	return &Resolver{
		input:    in,
		output:   out,
		template: template,
	}
}

func lookup(name string) Response {
	start := time.Now()
	addrs, err := net.LookupHost(name)
	res := Response{
		Item:      name,
		Duration:  time.Since(start),
		Addresses: addrs,
		Error:     err,
	}

	return res
}

// Run runs a resolver, processing requests from the input channel.
func (r *Resolver) Run(ctx context.Context) {
	for item := range r.input {
		name := strings.Replace(r.template, "FUZZ", item, -1)
		res := lookup(name)

		select {
		case <-ctx.Done():
			return
		case r.output <- res:
		}
	}
}
