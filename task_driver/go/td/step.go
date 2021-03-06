package td

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	multierror "github.com/hashicorp/go-multierror"
	"github.com/pborman/uuid"
	"go.skia.org/infra/go/exec"
	"go.skia.org/infra/go/sklog"
	"go.skia.org/infra/go/util"
	fsnotify "gopkg.in/fsnotify.v1"
)

const (
	MAX_STEP_NAME_CHARS = 100

	STEP_RESULT_SUCCESS   StepResult = "SUCCESS"
	STEP_RESULT_FAILURE   StepResult = "FAILURE"
	STEP_RESULT_EXCEPTION StepResult = "EXCEPTION"
)

// StepResult represents the result of a Step.
type StepResult string

// Start a new step, returning a context.Context associated with it.
func newStep(ctx context.Context, id string, parent *StepProperties, props *StepProperties) context.Context {
	if props == nil {
		props = &StepProperties{}
	}
	props.Id = id
	if parent != nil {
		// If empty, steps inherit their environment from their parent
		// step.
		// TODO(borenet): Should we merge environments?
		if len(props.Environ) == 0 {
			props.Environ = parent.Environ
		}

		// Steps inherit the infra status of their parent.
		// TODO(borenet): What if we want to have a parent which is an
		// infra step but a child which is not?
		if parent.IsInfra {
			props.IsInfra = true
		}

		props.Parent = parent.Id
	}
	ctx = setStep(ctx, props)
	ctx = execCtx(ctx)
	getRun(ctx).Start(props)
	return ctx
}

// Create a step.
func StartStep(ctx context.Context, props *StepProperties) context.Context {
	parent := getStep(ctx)
	return newStep(ctx, uuid.New(), parent, props)
}

// infraErrors collects all infrastructure errors.
var infraErrors = map[error]bool{}
var infraErrorsMtx sync.Mutex

// IsInfraError returns true if the given error is an infrastructure error.
func IsInfraError(err error) bool {
	infraErrorsMtx.Lock()
	defer infraErrorsMtx.Unlock()
	return infraErrors[err]
}

// InfraError wraps the given error, indicating that it is an infrastructure-
// related error. If the given error is already an InfraError, returns it as-is.
func InfraError(err error) error {
	infraErrorsMtx.Lock()
	defer infraErrorsMtx.Unlock()
	infraErrors[err] = true
	return err
}

// Mark the step as failed, with the given error. Returns the passed-in error
// for convenience, so that the caller can do things like:
//
//	if err := doSomething(); err != nil {
//		return FailStep(ctx, err)
//	}
//
func FailStep(ctx context.Context, err error) error {
	props := getStep(ctx)
	if props.IsInfra {
		err = InfraError(err)
	}
	getRun(ctx).Failed(props.Id, err)
	return err
}

// Mark the Step as finished. This is intended to be used in a defer, eg.
//
//	ctx = td.StartStep(ctx)
//	defer td.EndStep(ctx)
//
// If a panic is recovered in EndStep, the step is marked as failed and the
// panic is re-raised.
func EndStep(ctx context.Context) {
	finishStep(ctx, recover())
}

// finishStep is a helper function for EndStep which is also used by
// RunFinished to set the result of the root step.
func finishStep(ctx context.Context, recovered interface{}) {
	props := getStep(ctx)
	e := getRun(ctx)
	if recovered != nil {
		// If the panic is an error, use the original error, otherwise
		// create an error.
		err, ok := recovered.(error)
		if !ok {
			err = InfraError(fmt.Errorf("Caught panic: %v", recovered))
		}
		e.Failed(props.Id, err)
		defer panic(recovered)
	}
	e.Finish(props.Id)
}

// Attach the given StepData to this Step.
func StepData(ctx context.Context, typ DataType, d interface{}) {
	props := getStep(ctx)
	getRun(ctx).AddStepData(props.Id, typ, d)
}

// Do is a convenience function which runs the given function as a Step. It
// handles creation of the sub-step and calling EndStep() for you.
func Do(ctx context.Context, props *StepProperties, fn func(context.Context) error) error {
	ctx = StartStep(ctx, props)
	defer EndStep(ctx)
	if err := fn(ctx); err != nil {
		return FailStep(ctx, err)
	}
	return nil
}

// Fatal is a substitute for sklog.Fatal which logs an error and panics.
// sklog.Fatal does not panic but calls os.Exit, which prevents the Task Driver
// from properly reporting errors.
func Fatal(ctx context.Context, err error) {
	sklog.Error(err)
	if getStep(ctx).IsInfra {
		err = InfraError(err)
	}
	panic(err)
}

// Fatalf is a substitute for sklog.Fatalf which logs an error and panics.
// sklog.Fatalf does not panic but calls os.Exit, which prevents the Task Driver
// from properly reporting errors.
func Fatalf(ctx context.Context, format string, a ...interface{}) {
	Fatal(ctx, fmt.Errorf(format, a...))
}

// LogData is extra Step data generated for log streams.
type LogData struct {
	Name     string `json:"name"`
	Id       string `json:"id"`
	Severity string `json:"severity"`
	Log      string `json:"log,omitempty"`
}

// Create an io.Writer that will act as a log stream for this Step. Callers
// probably want to use a higher-level method instead.
func NewLogStream(ctx context.Context, name, severity string) io.Writer {
	props := getStep(ctx)
	return getRun(ctx).LogStream(props.Id, name, severity)
}

// FileStream is a struct used for streaming logs from a file, eg. when a test
// program writes verbose logs to a file. Intended to be used like this:
//
//	fs := s.NewFileStream("verbose")
//	defer util.Close(fs)
//	_, err := s.RunCwd(".", myTestProg, "--verbose", fs.FilePath())
//
type FileStream struct {
	cancel  context.CancelFunc
	ctx     context.Context
	doneCh  <-chan struct{}
	err     *multierror.Error
	file    *os.File
	name    string
	w       io.Writer
	watcher *fsnotify.Watcher
}

// Create a log stream which uses an intermediate file, eg. for writing from a
// test program.
func NewFileStream(ctx context.Context, name, severity string) (*FileStream, error) {
	w := NewLogStream(ctx, name, severity)
	f, err := ioutil.TempFile("", "log")
	if err != nil {
		return nil, fmt.Errorf("Failed to create file-based log stream; failed to create log file: %s", err)
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("Failed to create file-based log stream; failed to create fsnotify.Watcher: %s", err)
	}
	if err := watcher.Add(f.Name()); err != nil {
		return nil, fmt.Errorf("Failed to create file-based log stream; failed to add a watcher for the log file: %s", err)
	}
	ctx, cancel := context.WithCancel(ctx)
	doneCh := make(chan struct{})
	rv := &FileStream{
		cancel:  cancel,
		ctx:     ctx,
		doneCh:  doneCh,
		file:    f,
		name:    name,
		w:       w,
		watcher: watcher,
	}

	// Start collecting logs from the file.
	go rv.follow(doneCh)
	return rv, nil
}

// Read from the file incrementally as it is written, writing its contents to
// the step's log emitter.
func (fs *FileStream) follow(doneCh chan<- struct{}) {
	reportErr := func(format string, args ...interface{}) {
		err := fmt.Errorf(format, args...)
		getRun(fs.ctx).Failed(getStep(fs.ctx).Id, InfraError(err))
		fs.err = multierror.Append(fs.err, err)
	}

	defer func() {
		// Cleanup.
		if err := fs.file.Close(); err != nil {
			reportErr("Failed to close logstream file: %s", err)
		}
		if err := fs.watcher.Close(); err != nil {
			reportErr("Failed to close logstream file watcher: %s", err)
		}
		if err := os.Remove(fs.file.Name()); err != nil {
			reportErr("Failed to delete logstream file: %s", err)
		}
		doneCh <- struct{}{}
	}()

	buf := make([]byte, 128)
	for {
		select {
		case <-fs.ctx.Done():
			// fs.Close() was called; return.
			return
		case <-fs.watcher.Events:
			// The file was modified in some way; continue reading,
			// assuming that it was appended.
			for {
				nRead, err := fs.file.Read(buf)
				// Technically, an io.Reader is allowed to return
				// non-zero number of bytes read AND io.EOF on the
				// same call to Read(). Don't handle EOF until we've
				// written all of the data we read.
				if err != nil && err != io.EOF {
					reportErr("Failed to read from logstream file: %s", err)
					return
				}
				if nRead > 0 {
					nWrote, err := fs.w.Write(buf[:nRead])
					if err != nil {
						reportErr("Failed to write to log stream: %s", err)
						return
					}
					if nWrote != nRead {
						reportErr("Read %d bytes but wrote %d!", nRead, nWrote)
						return
					}
				}
				if err == io.EOF {
					break
				}
			}
		case err := <-fs.watcher.Errors:
			reportErr("fsnotify watcher error: %s", err)
			return
		}
	}
}

// Close the FileStream, cleaning up its resources and deleting the log file.
func (fs *FileStream) Close() error {
	fs.cancel()
	_ = <-fs.doneCh
	return fs.err.ErrorOrNil()
}

// Return the path to the logfile used by this FileStream.
func (fs *FileStream) FilePath() string {
	return fs.file.Name()
}

// ExecData is extra Step data generated when executing commands through the
// exec package.
type ExecData struct {
	Cmd []string `json:"command"`
	Env []string `json:"env,omitempty"`
}

// Return a context.Context associated with this Step. Any calls to exec which
// use this Context will be attached to the Step.
func execCtx(ctx context.Context) context.Context {
	return exec.NewContext(ctx, func(cmd *exec.Command) error {
		name := strings.Join(append([]string{cmd.Name}, cmd.Args...), " ")
		return Do(ctx, Props(name), func(ctx context.Context) error {
			props := getStep(ctx)
			// Inherit env from the step unless it's explicitly provided.
			// TODO(borenet): Should we merge instead?
			if len(cmd.Env) == 0 {
				cmd.Env = props.Environ
			}

			// Set up stdout and stderr streams.
			stdout := NewLogStream(ctx, "stdout", sklog.INFO)
			if cmd.Stdout != nil {
				stdout = util.MultiWriter([]io.Writer{cmd.Stdout, stdout})
			}
			cmd.Stdout = stdout
			stderr := NewLogStream(ctx, "stderr", sklog.ERROR)
			if cmd.Stderr != nil {
				stderr = util.MultiWriter([]io.Writer{cmd.Stderr, stderr})
			}
			cmd.Stderr = stderr

			// Collect step metadata about the command.
			d := &ExecData{
				Cmd: append([]string{cmd.Name}, cmd.Args...),
				Env: cmd.Env,
			}
			StepData(ctx, DATA_TYPE_COMMAND, d)

			// Run the command.
			return exec.DefaultRun(cmd)
		})
	})
}

// httpTransport is an http.RoundTripper which wraps another http.RoundTripper
// to record data about the requests it sends.
type httpTransport struct {
	ctx context.Context
	rt  http.RoundTripper
}

// HttpRequestData is Step data describing an http.Request. Notably, it does not
// include the request body or headers, to avoid leaking auth tokens or other
// sensitive information.
type HttpRequestData struct {
	Method string   `json:"method,omitempty"`
	URL    *url.URL `json:"url,omitempty"`
}

// HttpResponseData is Step data describing an http.Response. Notably, it does
// not include the response body, to avoid leaking sensitive information.
type HttpResponseData struct {
	StatusCode int `json:"status,omitempty"`
}

// See documentation for http.RoundTripper.
func (t *httpTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var resp *http.Response
	return resp, Do(t.ctx, Props(req.URL.String()), func(ctx context.Context) error {
		StepData(ctx, DATA_TYPE_HTTP_REQUEST, &HttpRequestData{
			Method: req.Method,
			URL:    req.URL,
		})
		var err error
		resp, err = t.rt.RoundTrip(req)
		if resp != nil {
			StepData(ctx, DATA_TYPE_HTTP_RESPONSE, &HttpResponseData{
				StatusCode: resp.StatusCode,
			})
		}
		return err
	})
}

// Return an http.Client which wraps the given http.Client to record data about
// the requests it sends.
func HttpClient(ctx context.Context, c *http.Client) *http.Client {
	if c == nil {
		c = http.DefaultClient // TODO(borenet): Use backoff client?
	}
	if c.Transport == nil {
		c.Transport = http.DefaultTransport
	}
	c.Transport = &httpTransport{
		ctx: ctx,
		rt:  c.Transport,
	}
	return c
}
