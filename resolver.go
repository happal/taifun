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
	input        <-chan string
	output       chan<- Result
	requestTypes []string

	template string
	server   string
}

// FindSystemNameserver returns a name server configured for the system.
func FindSystemNameserver() (string, error) {
	var nameserver string
	var once sync.Once
	wantError := errors.New("findSystemResolver")

	resolver := &net.Resolver{
		// do not use the cgo resolver so we can get the IP address of the default nameserver
		PreferGo: true,

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
func NewResolver(in <-chan string, out chan<- Result, template string, server string, requestTypes []string) (*Resolver, error) {
	if server == "" {
		return nil, errors.New("nameserver not specified")
	}

	res := &Resolver{
		input:        in,
		output:       out,
		template:     template,
		server:       server,
		requestTypes: requestTypes,
	}
	return res, nil
}

// cleanHostname removes a trailing dot if present.
func cleanHostname(h string) string {
	if h == "" {
		return h
	}
	last := len(h) - 1
	if h[last] == '.' {
		return h[:last]
	}
	return h
}

func collectRawValues(list []dns.RR) (records []string) {
	for _, item := range list {
		records = append(records, strings.Replace(item.String(), "\t", " ", -1))
	}
	return records
}

func sendRequest(name, item, requestType, server string) (request Request) {
	request = Request{
		Type: requestType,
	}

	c := dns.Client{}
	m := dns.Msg{}
	reqType := dns.StringToType[requestType]

	m.SetQuestion(name, reqType)

	res, _, err := c.Exchange(&m, net.JoinHostPort(server, "53"))
	if err != nil {
		request.Error = err
		return request
	}

	request.Status = dns.RcodeToString[res.MsgHdr.Rcode]
	if res.MsgHdr.Rcode != dns.RcodeSuccess {
		request.Failure = true
	}

	if request.Status == "NXDOMAIN" {
		request.NotFound = true
	}

	for _, ans := range res.Answer {
		// disregard additional data we did not ask for
		if ans.Header().Name != res.Question[0].Name {
			continue
		}

		if rec, ok := ans.(*dns.A); ok {
			request.Responses = append(request.Responses, NewResponse("A", rec.Header().Ttl, rec.A.String()))
		}
		if rec, ok := ans.(*dns.AAAA); ok {
			request.Responses = append(request.Responses, NewResponse("AAAA", rec.Header().Ttl, rec.AAAA.String()))
		}
		if rec, ok := ans.(*dns.CNAME); ok {
			request.Responses = append(request.Responses, NewResponse("CNAME", rec.Header().Ttl, cleanHostname(rec.Target)))
		}
		if rec, ok := ans.(*dns.MX); ok {
			request.Responses = append(request.Responses, NewResponse("MX", rec.Header().Ttl, cleanHostname(rec.Mx)))
		}
		if rec, ok := ans.(*dns.PTR); ok {
			request.Responses = append(request.Responses, NewResponse("PTR", rec.Header().Ttl, cleanHostname(rec.Ptr)))
		}
	}

	// collect nameservers in case of delegated sub domains
	for _, ans := range res.Ns {
		if rec, ok := ans.(*dns.SOA); ok {
			if rec.Hdr.Name == name {
				request.SOA = append(request.SOA, NewResponse("SOA", rec.Header().Ttl, cleanHostname(rec.Ns)))
			}
		}
		if rec, ok := ans.(*dns.NS); ok {
			if rec.Hdr.Name == name {
				request.Nameserver = append(request.Nameserver, NewResponse("NS", rec.Header().Ttl, cleanHostname(rec.Ns)))
			}
		}
	}

	// collect the raw responses
	for _, q := range res.Question {
		request.Raw.Question = append(request.Raw.Question, strings.Replace(q.String()[1:], "\t", " ", -1))
	}
	request.Raw.Answer = collectRawValues(res.Answer)
	request.Raw.Extra = collectRawValues(res.Extra)
	request.Raw.Nameserver = collectRawValues(res.Ns)

	return request
}

func (r *Resolver) lookup(ctx context.Context, item string) Result {
	name := strings.Replace(r.template, "FUZZ", item, -1)

	result := Result{
		Hostname: cleanHostname(name),
		Item:     item,
	}

	for _, requestType := range r.requestTypes {
		result.Requests = append(result.Requests, sendRequest(name, item, requestType, r.server))
	}

	return result
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
