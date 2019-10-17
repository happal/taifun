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

	*net.Resolver
}

// NewResolver returns a new resolver with the given input and output channels.
func NewResolver(in <-chan string, out chan<- Response, template string, server string) *Resolver {
	resolver := &net.Resolver{
		PreferGo:     true,
		StrictErrors: true,
	}

	if server != "" {
		// use the provided server, not the system DNS resolver by setting the
		// Dial method to always use the provided server.
		dialer := net.Dialer{}
		resolver.Dial = func(ctx context.Context, network, address string) (net.Conn, error) {
			return dialer.DialContext(ctx, "udp", net.JoinHostPort(server, "53"))
		}
	}

	return &Resolver{
		input:    in,
		output:   out,
		template: template,
		Resolver: resolver,
	}
}

func (r *Resolver) lookup(ctx context.Context, item string) Response {
	name := strings.Replace(r.template, "FUZZ", item, -1)
	start := time.Now()
	addrs, err := r.LookupHost(ctx, name)
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
		res := r.lookup(ctx, item)

		select {
		case <-ctx.Done():
			return
		case r.output <- res:
		}
	}
}
