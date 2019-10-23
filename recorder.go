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
	HiddenResults int       `json:"hidden_responses"`
	ShownResults  int       `json:"shown_responses"`
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

	Responses map[string]RecordedResponse `json:"responses"`
}

// RecordedResponse is the result of a request.
type RecordedResponse struct {
	Status    string   `json:"status"`
	Addresses []string `json:"addresses,omitempty"`
	CNAMEs    []string `json:"cnames,omitempty"`
	Error     string   `json:"error,omitempty"`
}

// NewRecorder creates a new  recorder.
func NewRecorder(filename string, hostname string) (*Recorder, error) {
	rec := &Recorder{
		filename: filename,
		Data: Data{
			Hostname: hostname,
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
			data.Results = append(data.Results, NewResult(res))
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

// NewRecordedResponse creates a response for JSON encoding from a Response.
func NewRecordedResponse(r Response) RecordedResponse {
	res := RecordedResponse{
		Status:    r.Status,
		Addresses: r.Addresses,
		CNAMEs:    r.CNAMEs,
	}

	if r.Error != nil {
		res.Error = r.Error.Error()
	}

	return res
}

// NewResult builds a Result struct for serialization with JSON.
func NewResult(r Result) (res RecordedResult) {
	res.Item = r.Item
	res.Hostname = r.Hostname

	res.Responses = make(map[string]RecordedResponse)
	if !r.A.Empty() {
		res.Responses["A"] = NewRecordedResponse(r.A)
	}
	if !r.AAAA.Empty() {
		res.Responses["AAAA"] = NewRecordedResponse(r.AAAA)
	}

	return res
}
