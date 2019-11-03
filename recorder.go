package main

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"time"
)

// Recorder records information about received responses in a file encoded as JSON.
type Recorder struct {
	filename string
	Data
}

// Data is the data structure written to the file by a Recorder.
type Data struct {
	Start         time.Time `json:"start"`
	End           time.Time `json:"end"`
	TotalRequests int       `json:"total_requests"`
	SentRequests  int       `json:"sent_requests"`
	HiddenResults int       `json:"hidden_results"`
	ShownResults  int       `json:"shown_results"`
	Cancelled     bool      `json:"cancelled"`

	Hostname    string           `json:"hostname"`
	InputFile   string           `json:"input_file,omitempty"`
	Range       string           `json:"range,omitempty"`
	RangeFormat string           `json:"range_format,omitempty"`
	Results     []RecordedResult `json:"responses"`
}

// RecordedResult is the result of a request sent to the target.
type RecordedResult struct {
	Item     string `json:"item"`
	Hostname string `json:"hostname"`

	PotentialSuffix     bool     `json:"potential_prefix,omitempty"`
	PotentialDelegation bool     `json:"potential_delegation,omitempty"`
	Nameservers         []string `json:"nameservers,omitempty"`

	Requests []RecordedRequest `json:"requests"`
}

// RecordedRequest captures one particular request.
type RecordedRequest struct {
	Error string `json:"error,omitempty"`

	Type      string              `json:"type"`
	Status    string              `json:"status"`
	Responses []RecordedResponse  `json:"responses,omitempty"`
	Raw       RawRecordedResponse `json:"raw"`
}

// RecordedResponse is a serialized response.
type RecordedResponse struct {
	Type string `json:"type"`
	Data string `json:"data"`

	TTL uint `json:"ttl"`
}

// RawRecordedResponse contains the (string versions of) the raw DNS response.
type RawRecordedResponse struct {
	Question   []string `json:"question,omitempty"`
	Answer     []string `json:"answer,omitempty"`
	Nameserver []string `json:"nameserver,omitempty"`
	Extra      []string `json:"extra,omitempty"`
}

// NewRecorder creates a new  recorder.
func NewRecorder(filename string, hostname string) (*Recorder, error) {
	rec := &Recorder{
		filename: filename,
		Data: Data{
			Hostname: hostname,
			Results:  []RecordedResult{},
		},
	}
	return rec, nil
}

const statusInterval = time.Second

// Run reads responses from ch and forwards them to the returned channel,
// recording statistics on the way. When ch is closed or the context is
// cancelled, the output file is closed, processing stops, and the output
// channel is closed.
func (r *Recorder) Run(ctx context.Context, in <-chan Result, out chan<- Result, inCount <-chan int, outCount chan<- int) error {
	defer close(out)

	data := r.Data
	data.Start = time.Now()
	data.End = time.Now()

	// omit range_format if range is unset
	if data.Range == "" {
		data.RangeFormat = ""
	}

	lastStatus := time.Now()

	var countCh chan<- int // countCh is nil initially to disable sending

loop:
	for {
		var res Result
		var ok bool

		select {
		case <-ctx.Done():
			data.Cancelled = true
			break loop

		case res, ok = <-in:
			if !ok {
				// we're done, exit
				break loop
			}

		case total := <-inCount:
			data.TotalRequests = total
			// disable receiving on the in count channel
			inCount = nil
			// enable sending by setting countCh to outCount (which is not nil)
			countCh = outCount
			continue loop

		case countCh <- data.TotalRequests:
			// disable sending again by setting countCh to nil
			countCh = nil
			continue loop
		}

		data.SentRequests++
		if !res.Hide {
			data.ShownResults++
			rres := NewResult(res)
			if !rres.Empty() {
				data.Results = append(data.Results, rres)
			}
		} else {
			data.HiddenResults++
		}

		data.End = time.Now()

		if time.Since(lastStatus) > statusInterval {
			lastStatus = time.Now()

			err := r.dump(data)
			if err != nil {
				return err
			}
		}

		select {
		case <-ctx.Done():
			data.Cancelled = true
			break loop
		case out <- res:
		}
	}

	data.End = time.Now()
	return r.dump(data)
}

// dump writes the current status to the file.
func (r *Recorder) dump(data Data) error {
	buf, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	buf = append(buf, '\n')

	return ioutil.WriteFile(r.filename, buf, 0644)
}

// NewResult builds a Result struct for serialization with JSON.
func NewResult(r Result) (res RecordedResult) {
	res = RecordedResult{
		Item:     r.Item,
		Hostname: r.Hostname,
		Requests: []RecordedRequest{},
	}

	if r.Delegation() {
		res.PotentialDelegation = true
		res.Nameservers = r.Nameservers()
		return res
	}

	if r.Empty() {
		res.PotentialSuffix = true
		return res
	}

	for _, request := range r.Requests {
		// do not record hidden requests
		if request.Hide || request.Empty() {
			continue
		}
		req := RecordedRequest{
			Status: request.Status,
			Type:   request.Type,
			Raw:    RawRecordedResponse(request.Raw),
		}
		if request.Error != nil {
			req.Error = request.Error.Error()
		}

		for _, response := range request.Responses {
			// do not record hidden responses
			if response.Hide {
				continue
			}

			req.Responses = append(req.Responses, RecordedResponse{
				Type: response.Type,
				Data: response.Data,
				TTL:  response.TTL,
			})
		}

		if len(req.Responses) == 0 {
			continue
		}

		res.Requests = append(res.Requests, req)
	}

	return res
}

// Empty returns true if the responses are all hidden or empty.
func (r RecordedResult) Empty() bool {
	if len(r.Requests) > 0 {
		return false
	}

	if r.PotentialSuffix || r.PotentialDelegation {
		return false
	}

	return true
}
