package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/miekg/dns"
)

// Resolver executes DNS requests.
type Resolver struct {
	input  <-chan string
	output chan<- Response

	template string
	server   string
}

// FindSystemNameserver returns a name server configured for the system.
func FindSystemNameserver() (string, error) {
	var nameserver string
	var once sync.Once
	wantError := errors.New("findSystemResolver")

	resolver := &net.Resolver{
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return nil, fmt.Errorf("unable to find system nameserver, split failed: %v", err)
			}
			once.Do(func() {
				nameserver = host
			})
			return nil, wantError
		},
	}

	_, err := resolver.LookupHost(context.Background(), "example.com")
	if dnsError, ok := err.(*net.DNSError); ok {
		if dnsError.Err == wantError.Error() {
			return nameserver, nil
		}
	}

	return "", errors.New("unable to find system nameserver, please specify a server manually")
}

// NewResolver returns a new resolver with the given input and output channels.
func NewResolver(in <-chan string, out chan<- Response, template string, server string) (*Resolver, error) {
	if server == "" {
		return nil, errors.New("nameserver not specified")
	}

	res := &Resolver{
		input:    in,
		output:   out,
		template: template,
		server:   server,
	}
	return res, nil
}

func (r *Resolver) lookup(ctx context.Context, item string) Response {
	name := strings.Replace(r.template, "FUZZ", item, -1)

	c := dns.Client{}
	m := dns.Msg{}
	m.SetQuestion(name, dns.TypeA)

	response := Response{
		Hostname: name,
		Item:     item,
	}

	res, duration, err := c.Exchange(&m, r.server+":53")
	response.Duration = duration
	response.Error = err
	if err != nil {
		return response
	}

	response.Status = dns.RcodeToString[res.MsgHdr.Rcode]
	if res.MsgHdr.Rcode != dns.RcodeSuccess {
		response.Failure = true
	}

	for _, ans := range res.Answer {
		// disregard additional data we did not ask for
		if ans.Header().Name != res.Question[0].Name {
			continue
		}

		if rec, ok := ans.(*dns.A); ok {
			response.Addresses = append(response.Addresses, rec.A.String())
			continue
		}
		if rec, ok := ans.(*dns.AAAA); ok {
			response.Addresses = append(response.Addresses, rec.AAAA.String())
			continue
		}
		if rec, ok := ans.(*dns.CNAME); ok {
			response.CNAMES = append(response.CNAMES, strings.TrimRight(rec.Target, "."))
			continue
		}
	}

	return response
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
