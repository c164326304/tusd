package tusd

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
)

// Metrics provides numbers about the usage of the tusd handler. Since these may
// be accessed from multiple goroutines, it is necessary to read and modify them
// atomically using the functions exposed in the sync/atomic package, such as
// atomic.LoadUint64. In addition the maps must not be modified to prevent data
// races.
type Metrics struct {
	// RequestTotal counts the number of incoming requests per method
	RequestsTotal map[string]*uint64
	// ErrorsTotal counts the number of returned errors by their message
	ErrorsTotal       *ErrorsTotalMap
	BytesReceived     *uint64
	UploadsFinished   *uint64
	UploadsCreated    *uint64
	UploadsTerminated *uint64
}

// incRequestsTotal increases the counter for this request method atomically by
// one. The method must be one of GET, HEAD, POST, PATCH, DELETE.
func (m Metrics) incRequestsTotal(method string) {
	if ptr, ok := m.RequestsTotal[method]; ok {
		atomic.AddUint64(ptr, 1)
	}
}

// incErrorsTotal increases the counter for this error atomically by one.
func (m Metrics) incErrorsTotal(err HTTPError) {
	ptr := m.ErrorsTotal.retrievePointerFor(err)
	atomic.AddUint64(ptr, 1)
}

// incBytesReceived increases the number of received bytes atomically be the
// specified number.
func (m Metrics) incBytesReceived(delta uint64) {
	atomic.AddUint64(m.BytesReceived, delta)
}

// incUploadsFinished increases the counter for finished uploads atomically by one.
func (m Metrics) incUploadsFinished() {
	atomic.AddUint64(m.UploadsFinished, 1)
}

// incUploadsCreated increases the counter for completed uploads atomically by one.
func (m Metrics) incUploadsCreated() {
	atomic.AddUint64(m.UploadsCreated, 1)
}

// incUploadsTerminated increases the counter for completed uploads atomically by one.
func (m Metrics) incUploadsTerminated() {
	atomic.AddUint64(m.UploadsTerminated, 1)
}

func newMetrics() Metrics {
	return Metrics{
		RequestsTotal: map[string]*uint64{
			"GET":     new(uint64),
			"HEAD":    new(uint64),
			"POST":    new(uint64),
			"PATCH":   new(uint64),
			"DELETE":  new(uint64),
			"OPTIONS": new(uint64),
		},
		ErrorsTotal:       newErrorsTotalMap(),
		BytesReceived:     new(uint64),
		UploadsFinished:   new(uint64),
		UploadsCreated:    new(uint64),
		UploadsTerminated: new(uint64),
	}
}

// ErrorsTotalMap stores the counters for the different HTTP errors.
type ErrorsTotalMap struct {
	lock    sync.RWMutex
	counter map[simpleHTTPError]*uint64
}

type simpleHTTPError struct {
	Message    string
	StatusCode int
}

func simplifyHTTPError(err HTTPError) simpleHTTPError {
	var msg string
	// Errors for read timeouts contain too much information which is not
	// necessary for us and makes grouping for the metrics harder. The error
	// message looks like: read tcp 127.0.0.1:1080->127.0.0.1:53673: i/o timeout
	// Therefore, we use a common error message for all of them.
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		msg = "read tcp: i/o timeout"
	} else {
		msg = err.Error()
	}
	return simpleHTTPError{
		Message:    msg,
		StatusCode: err.StatusCode(),
	}
}

func newErrorsTotalMap() *ErrorsTotalMap {
	m := make(map[simpleHTTPError]*uint64, 20)
	return &ErrorsTotalMap{
		counter: m,
	}
}

// retrievePointerFor returns (after creating it if necessary) the pointer to
// the counter for the error.
func (e *ErrorsTotalMap) retrievePointerFor(err HTTPError) *uint64 {
	serr := simplifyHTTPError(err)
	e.lock.RLock()
	ptr, ok := e.counter[serr]
	e.lock.RUnlock()
	if ok {
		return ptr
	}

	// For pointer creation, a write-lock is required
	e.lock.Lock()
	// We ensure that the pointer wasn't created in the meantime
	if ptr, ok = e.counter[serr]; !ok {
		ptr = new(uint64)
		e.counter[serr] = ptr
	}
	e.lock.Unlock()

	return ptr
}

// Load retrieves the map of the counter pointers atomically
func (e *ErrorsTotalMap) Load() map[HTTPError]*uint64 {
	m := make(map[HTTPError]*uint64, len(e.counter))
	e.lock.RLock()
	for err, ptr := range e.counter {
		httpErr := NewHTTPError(errors.New(err.Message), err.StatusCode)
		m[httpErr] = ptr
	}
	e.lock.RUnlock()

	return m
}
