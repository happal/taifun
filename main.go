package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fd0/termstatus"
	"github.com/happal/hagel/cli"
	"github.com/happal/hagel/producer"
	"github.com/happal/hagel/shell"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

// Options collect global options for the program.
type Options struct {
	Range       string
	RangeFormat string
	Filename    string

	BufferSize int
	Skip       int
	Limit      int

	Logfile string
	Logdir  string
	Threads int

	Nameserver string

	RequestsPerSecond float64

	ShowNotFound bool
}

func (opts *Options) valid() (err error) {
	if opts.Threads <= 0 {
		return errors.New("invalid number of threads")
	}

	if opts.Range != "" && opts.Filename != "" {
		return errors.New("only one source allowed but both range and filename specified")
	}

	if opts.Range == "" && opts.Filename == "" {
		return errors.New("neither file nor range specified, nothing to do")
	}

	return nil
}

// logfilePath returns the prefix for the logfiles, if any.
func logfilePath(opts *Options, hostname string) (prefix string, err error) {
	if opts.Logdir != "" && opts.Logfile == "" {
		ts := time.Now().Format("20060102_150405")
		fn := fmt.Sprintf("hagel_%s_%s", hostname, ts)
		p := filepath.Join(opts.Logdir, fn)
		return p, nil
	}

	return opts.Logfile, nil
}

func setupTerminal(ctx context.Context, g *errgroup.Group, logfilePrefix string) (term cli.Terminal, cleanup func(), err error) {
	ctx, cancel := context.WithCancel(context.Background())

	if logfilePrefix != "" {
		fmt.Printf("logfile is %s.log\n", logfilePrefix)

		logfile, err := os.Create(logfilePrefix + ".log")
		if err != nil {
			return nil, cancel, err
		}

		fmt.Fprintln(logfile, shell.Join(os.Args))

		// write copies of messages to logfile
		term = &cli.LogTerminal{
			Terminal: termstatus.New(os.Stdout, os.Stderr, false),
			Writer:   logfile,
		}
	} else {
		term = termstatus.New(os.Stdout, os.Stderr, false)
	}

	// make sure error messages logged via the log package are printed nicely
	w := cli.NewStdioWrapper(term)
	log.SetOutput(w.Stderr())

	g.Go(func() error {
		term.Run(ctx)
		return nil
	})

	return term, cancel, nil
}

func setupProducer(ctx context.Context, g *errgroup.Group, opts *Options, ch chan<- string, count chan<- int) error {
	switch {
	case opts.Range != "":
		var first, last int
		_, err := fmt.Sscanf(opts.Range, "%d-%d", &first, &last)
		if err != nil {
			return errors.New("wrong format for range, expected: first-last")
		}

		g.Go(func() error {
			return producer.Range(ctx, first, last, opts.RangeFormat, ch, count)
		})
		return nil

	case opts.Filename == "-":
		g.Go(func() error {
			return producer.Reader(ctx, os.Stdin, ch, count)
		})
		return nil

	case opts.Filename != "":
		file, err := os.Open(opts.Filename)
		if err != nil {
			return err
		}

		g.Go(func() error {
			return producer.Reader(ctx, file, ch, count)
		})
		return nil

	default:
		return errors.New("neither file nor range specified, nothing to do")
	}
}

func setupValueFilters(ctx context.Context, opts *Options, valueCh <-chan string, countCh <-chan int) (<-chan string, <-chan int) {
	if opts.Skip > 0 {
		f := &producer.FilterSkip{Skip: opts.Skip}
		countCh = f.Count(ctx, countCh)
		valueCh = f.Select(ctx, valueCh)
	}

	if opts.Limit > 0 {
		f := &producer.FilterLimit{Max: opts.Limit}
		countCh = f.Count(ctx, countCh)
		valueCh = f.Select(ctx, valueCh)
	}

	return valueCh, countCh
}

func setupResponseFilters(opts *Options) (filters []Filter, err error) {
	if !opts.ShowNotFound {
		filters = append(filters, FilterNotFound())
	}
	return filters, nil
}

func startResolvers(ctx context.Context, opts *Options, hostname string, in <-chan string) (<-chan Response, error) {
	out := make(chan Response)

	var wg sync.WaitGroup
	for i := 0; i < opts.Threads; i++ {
		resolver := NewResolver(in, out, hostname, opts.Nameserver)
		wg.Add(1)
		go func() {
			resolver.Run(ctx)
			wg.Done()
		}()
	}

	go func() {
		// wait until the resolvers are done, then close the output channel
		wg.Wait()
		close(out)
	}()

	return out, nil
}

func run(ctx context.Context, g *errgroup.Group, opts *Options, args []string) error {
	if len(args) == 0 {
		return errors.New("last argument needs to be the host name")
	}

	if len(args) > 1 {
		return errors.New("more than one target host name specified")
	}

	hostname := args[0]

	if !strings.Contains(hostname, "FUZZ") {
		return errors.New(`hostname does not contain the string "FUZZ"`)
	}

	// make sure the hostname is absolute
	if !strings.HasSuffix(hostname, ".") {
		hostname += "."
	}

	err := opts.valid()
	if err != nil {
		return err
	}

	// setup logging and the terminal
	logfilePrefix, err := logfilePath(opts, hostname)
	if err != nil {
		return err
	}

	term, cleanup, err := setupTerminal(ctx, g, logfilePrefix)
	defer cleanup()
	if err != nil {
		return err
	}

	// collect the filters for the responses
	responseFilters, err := setupResponseFilters(opts)
	if err != nil {
		return err
	}

	// setup the pipeline for the values
	vch := make(chan string, opts.BufferSize)
	var valueCh <-chan string = vch
	cch := make(chan int, 1)
	var countCh <-chan int = cch

	// start a producer from the options
	err = setupProducer(ctx, g, opts, vch, cch)
	if err != nil {
		return err
	}

	// filter values (skip, limit)
	valueCh, countCh = setupValueFilters(ctx, opts, valueCh, countCh)

	// limit the throughput (if requested)
	if opts.RequestsPerSecond > 0 {
		valueCh = producer.Limit(ctx, opts.RequestsPerSecond, valueCh)
	}

	// start the resolvers
	responseCh, err := startResolvers(ctx, opts, hostname, valueCh)
	if err != nil {
		return err
	}

	// filter the responses
	responseCh = Mark(responseCh, responseFilters)

	if logfilePrefix != "" {
		rec, err := NewRecorder(logfilePrefix+".json", hostname)
		if err != nil {
			return err
		}

		// fill in information for generating the request
		rec.Data.InputFile = opts.Filename
		rec.Data.Range = opts.Range
		rec.Data.RangeFormat = opts.RangeFormat

		out := make(chan Response)
		in := responseCh
		responseCh = out

		outCount := make(chan int)
		inCount := countCh
		countCh = outCount

		g.Go(func() error {
			return rec.Run(ctx, in, out, inCount, outCount)
		})
	}

	// run the reporter
	term.Printf("hostname %v\n\n", hostname)
	reporter := NewReporter(term)
	return reporter.Display(responseCh, countCh)
}

func main() {
	var opts Options

	cmd := &cobra.Command{
		Use:                   "hagel [options] HOSTNAME",
		DisableFlagsInUseLine: true,
		SilenceErrors:         true,
		SilenceUsage:          true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cli.WithContext(func(ctx context.Context, g *errgroup.Group) error {
				return run(ctx, g, &opts, args)
			})
		},
	}

	flags := cmd.Flags()
	flags.IntVarP(&opts.Threads, "threads", "t", 5, "resolve `n` DNS queries in parallel")
	flags.Float64Var(&opts.RequestsPerSecond, "requests-per-second", 0, "do at most `n` requests per seconds (e.g. 0.5)")
	flags.IntVar(&opts.BufferSize, "buffer-size", 100000, "set number of buffered items to `n`")
	flags.StringVar(&opts.Logfile, "logfile", "", "write copy of printed messages to `filename`.log")
	flags.StringVar(&opts.Logdir, "logdir", os.Getenv("HAGEL_LOG_DIR"), "automatically log all output to files in `dir`")

	flags.IntVar(&opts.Skip, "skip", 0, "skip the first `n` requests")
	flags.IntVar(&opts.Limit, "limit", 0, "only run `n` requests, then exit")

	flags.StringVarP(&opts.Filename, "file", "f", "", "read values to test from `filename`")
	flags.StringVarP(&opts.Range, "range", "r", "", "test range `from-to`")
	flags.StringVar(&opts.RangeFormat, "range-format", "%d", "set `format` for range")

	flags.StringVar(&opts.Nameserver, "nameserver", "", "send DNS queries to `server`, if empty, the system resolver is used")

	flags.BoolVar(&opts.ShowNotFound, "show-not-found", false, "do not hide 'not found' responses")

	err := cmd.Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing options: %v\n", err)
		os.Exit(1)
	}
}
