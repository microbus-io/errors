/*
Copyright (c) 2023-2026 Microbus LLC and various contributors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package errors

import (
	stderrors "errors"
	"fmt"
	"maps"
	"reflect"
)

var statusText = map[int]string{
	// 1xx
	100: "continue",
	101: "switching protocol",
	102: "processing",
	103: "early hints",
	// 2xx
	200: "ok",
	201: "created",
	202: "accepted",
	203: "non-authoritative information",
	204: "no content",
	205: "reset content",
	206: "partial content",
	// 3xx
	300: "multiple choices",
	301: "moved permanently",
	302: "found",
	303: "see other",
	304: "not modified",
	307: "temporary redirect",
	308: "permanent redirect",
	// 4xx
	400: "bad request",
	401: "unauthorized",
	402: "payment required",
	403: "forbidden",
	404: "not found",
	405: "method not allowed",
	406: "not acceptable",
	407: "proxy authentication required",
	408: "request timeout",
	409: "conflict",
	410: "gone",
	411: "length required",
	412: "preconditions failed",
	413: "content too large",
	414: "uri too long",
	415: "unsupported media type",
	416: "range not satisfiable",
	417: "expectation failed",
	418: "i'm a teapot",
	421: "misdirected request",
	422: "unprocessable content",
	423: "locked",
	424: "failed dependency",
	425: "too early",
	426: "upgrade required",
	428: "precondition required",
	429: "too many requests",
	431: "request header fields too large",
	451: "unavailable for legal reasons",
	// 5xx
	500: "internal server error",
	501: "not implemented",
	502: "bad gateway",
	503: "service unavailable",
	504: "gateway timeout",
	505: "http version not supported",
	506: "variant also negotiates",
	507: "insufficient storage",
	508: "loop detected",
	510: "not extended",
	511: "network authentication required",
}

// As delegates to the standard Go's errors.As function.
func As(err error, target any) bool {
	return stderrors.As(err, target)
}

// Is delegates to the standard Go's errors.Is function.
func Is(err, target error) bool {
	return stderrors.Is(err, target)
}

// Join aggregates multiple errors into one and captures a new stack trace.
// If more than one of the errors is not nil, their status codes, trace IDs, properties and stack traces are discarded
// and the joined error has status code 500.
// If only one of the errors is not nil, it is traced as if with Trace, keeping its status code, trace ID, properties and
// stack trace.
func Join(errs ...error) error {
	var err error
	var n int
	for _, e := range errs {
		if e != nil {
			err = e
			n++
		}
	}
	switch n {
	case 0:
		return nil
	case 1:
		return Trace(err)
	default:
		return traceCaller(&TracedError{Err: stderrors.Join(errs...), StatusCode: 500})
	}
}

// Unwrap delegates to the standard Go's errors.Wrap function.
func Unwrap(err error) error {
	return stderrors.Unwrap(err)
}

// Trace appends the current stack location to the error's stack trace.
// The variadic arguments behave like those of New.
func Trace(err error, a ...any) error {
	if err == nil {
		return nil
	}
	return New("", append([]any{err}, a...)...)
}

// Convert converts an error to one that supports stack tracing.
// If the error already supports this, it is returned as it is, unless its status code is 0, in which case it is wrapped
// as described below so that the original is not modified.
// If the error wraps a TracedError further down its chain, such as one wrapped by another package's error type, it is
// converted to a new TracedError that wraps it and inherits the status code, trace ID, properties and stack trace of that
// TracedError. Only errors that wrap a single error are looked into. A chain that branches into more than one error, as
// errors.Join or fmt.Errorf with several %w can create, has no single TracedError to inherit from.
// Any other error, including a nil *TracedError held in a non-nil error, is converted to a new TracedError with status
// code 500.
// Note: Trace should be called to include the error's trace in the stack.
func Convert(err error) *TracedError {
	if err == nil {
		return nil
	}
	if tracedErr, ok := err.(*TracedError); ok && tracedErr != nil && tracedErr.StatusCode != 0 {
		return tracedErr
	}
	return wrap(err)
}

// wrap returns a new TracedError that wraps err.
// It inherits the status code, trace ID, properties and stack trace of the TracedError that findTraced finds in err's
// chain, if any, without sharing its properties or stack trace. The status code defaults to 500.
func wrap(err error) *TracedError {
	tracedErr := &TracedError{
		Err:        err,
		StatusCode: 500,
	}
	if inner := findTraced(err); inner != nil {
		if inner.StatusCode != 0 {
			tracedErr.StatusCode = inner.StatusCode
		}
		tracedErr.Trace = inner.Trace
		tracedErr.Properties = maps.Clone(inner.Properties)
		tracedErr.Stack = cloneStack(inner.Stack)
	}
	return tracedErr
}

// findTraced returns the TracedError at or below err in its chain of wrapped errors, or nil if there is none.
// Unlike As, it only looks into errors that wrap a single error. A chain that branches into more than one error, as
// errors.Join or fmt.Errorf with several %w can create, has no single TracedError to speak for it, so nil is returned.
// A nil pointer, such as a nil *url.Error, ends the chain, because its methods may not be safe to call.
func findTraced(err error) *TracedError {
	for err != nil && !isNilPointer(err) {
		switch x := err.(type) {
		case *TracedError:
			return x
		case interface{ As(any) bool }:
			var tracedErr *TracedError
			if x.As(&tracedErr) && tracedErr != nil {
				return tracedErr
			}
		}
		switch x := err.(type) {
		case interface{ Unwrap() error }:
			err = x.Unwrap()
		case interface{ Unwrap() []error }:
			errs := x.Unwrap()
			if len(errs) != 1 {
				return nil
			}
			err = errs[0]
		default:
			return nil
		}
	}
	return nil
}

// isNilPointer reports whether err holds a nil pointer, as in a nil *url.Error returned as an error.
func isNilPointer(err error) bool {
	v := reflect.ValueOf(err)
	return v.Kind() == reflect.Pointer && v.IsNil()
}

// StatusCode returns the HTTP status code associated with an error.
// It is the equivalent of Convert(err).StatusCode.
// The default status code is 500.
func StatusCode(err error) int {
	if err == nil {
		return 0
	}
	return Convert(err).StatusCode
}

/*
CatchPanic calls the given function and returns any panic as a standard error.

	err = errors.CatchPanic(func() error {
		panic("oops!")
		return nil
	})
*/
func CatchPanic(f func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok {
				err = e
			} else {
				err = fmt.Errorf("%v", r)
			}
			// Wrap the error so that a shared error, such as a sentinel, is not modified
			err = traceFull(wrap(err), 1)
		}
	}()
	err = f()
	return
}
