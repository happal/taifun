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

func lookup(template, item string) Response {
	name := strings.Replace(template, "FUZZ", item, -1)
	start := time.Now()
	addrs, err := net.LookupHost(name)
	res := Response{
		Hostname:  name,
		Item:      item,
		Duration:  time.Since(start),
		Addresses: addrs,
		Error:     err,
	}

	return res
}

// Run runs a resolver, processing requests from the input channel.
func (r *Resolver) Run(ctx context.Context) {
	for item := range r.input {
		res := lookup(r.template, item)

		select {
		case <-ctx.Done():
			return
		case r.output <- res:
		}
	}
}
