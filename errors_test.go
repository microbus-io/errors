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
	"encoding/json"
	stderrors "errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestErrors_TraceAfterNew(t *testing.T) {
	t.Parallel()

	err := New("error occurred", 409, "key", "value")
	err = Trace(err)
	tracedErr := Convert(err)
	assertEqual(t, "error occurred", tracedErr.Error())
	assertEqual(t, 409, tracedErr.StatusCode)
	assertEqual(t, "value", tracedErr.Properties["key"])
}

func TestErrors_New(t *testing.T) {
	t.Parallel()

	// Fixed error message
	err := New("fixed string")
	tracedErr := Convert(err)
	assertError(t, err)
	assertEqual(t, "fixed string", err.Error())
	assertEqual(t, 500, tracedErr.StatusCode)
	assertEqual(t, 0, len(tracedErr.Properties))
	assertEqual(t, 1, len(tracedErr.Stack))
	assertContains(t, tracedErr.Stack[0].Function, "TestErrors_New")

	// Formatted error message
	err = New("format string %s %d", "XYZ", 123)
	tracedErr = Convert(err)
	assertError(t, err)
	assertEqual(t, "format string XYZ 123", err.Error())
	assertEqual(t, 500, tracedErr.StatusCode)
	assertEqual(t, 0, len(tracedErr.Properties))
	assertEqual(t, 1, len(tracedErr.Stack))
	assertContains(t, tracedErr.Stack[0].Function, "TestErrors_New")

	// Formatted error message with arbitrary properties
	err = New("format string %s %d", "XYZ", 123,
		"strKey", "ABC",
		"intKey", 888,
	)
	tracedErr = Convert(err)
	assertError(t, err)
	assertEqual(t, "format string XYZ 123", err.Error())
	assertEqual(t, 500, tracedErr.StatusCode)
	assertEqual(t, 2, len(tracedErr.Properties))
	assertEqual(t, "ABC", tracedErr.Properties["strKey"])
	assertEqual(t, 888, tracedErr.Properties["intKey"])
	assertEqual(t, 1, len(tracedErr.Stack))
	assertContains(t, tracedErr.Stack[0].Function, "TestErrors_New")

	// Fixed error message with status code
	err = New("bad input", 400)
	assertError(t, err)
	assertEqual(t, "bad input", err.Error())
	assertEqual(t, 400, err.(*TracedError).StatusCode)
	assertEqual(t, 400, StatusCode(err))
	assertEqual(t, 1, len(err.(*TracedError).Stack))
	assertContains(t, err.(*TracedError).Stack[0].Function, "TestErrors_New")

	// Formatted error message with wrapped error and status code
	badDateStr := "2025-06-07T25:06:07Z"
	_, originalErr := time.Parse(time.RFC3339, badDateStr)
	err = New("failed to parse '%s'", badDateStr, originalErr, 400)
	tracedErr = Convert(err)
	assertError(t, err)
	assertEqual(t, "failed to parse '"+badDateStr+"': "+originalErr.Error(), err.Error())
	assertEqual(t, 400, tracedErr.StatusCode)
	assertEqual(t, 0, len(tracedErr.Properties))
	assertEqual(t, 1, len(tracedErr.Stack))
	assertContains(t, tracedErr.Stack[0].Function, "TestErrors_New")

	// Blank error message with wrapped error and status code
	err = New("", originalErr, 400)
	tracedErr = Convert(err)
	assertError(t, err)
	assertEqual(t, originalErr.Error(), err.Error())
	assertEqual(t, 400, tracedErr.StatusCode)

	// Blank error message with status code and wrapped error
	err = New("", 400, originalErr)
	tracedErr = Convert(err)
	assertError(t, err)
	assertEqual(t, "bad request: "+originalErr.Error(), err.Error())
	assertEqual(t, 400, tracedErr.StatusCode)

	// Blank error message with wrapped error
	err = New("", originalErr)
	assertError(t, err)
	assertEqual(t, originalErr.Error(), err.Error())

	// Traditional wrapping of error using %w
	err = New("failed to parse date: %w", originalErr)
	assertError(t, err)
	assertEqual(t, "failed to parse date: "+originalErr.Error(), err.Error())

	// Blank error message
	err = New("")
	assertError(t, err)
	assertNotEqual(t, "", err.Error())

	// Last status codes wins
	err = New("message", 5, 6, 7)
	assertError(t, err)
	tracedErr = Convert(err)
	assertEqual(t, 7, tracedErr.StatusCode)

	// Unnamed property
	err = New("message", false, "dur", time.Second)
	assertError(t, err)
	tracedErr = Convert(err)
	assertEqual(t, 2, len(tracedErr.Properties))
	assertEqual(t, false, tracedErr.Properties["!BADKEY"])

	// Not enough args for pattern
	err = New("pattern %s %d", "XYZ")
	assertError(t, err)
	assertContains(t, err.Error(), "pattern XYZ")

	// Double percent sign
	err = New("pattern %s 100%%d", "XYZ", 400)
	assertError(t, err)
	assertEqual(t, "pattern XYZ 100%d", err.Error())
	tracedErr = Convert(err)
	assertEqual(t, 400, tracedErr.StatusCode)
}

func TestErrors_Trace(t *testing.T) {
	t.Parallel()

	stdErr := stderrors.New("standard error")
	assertError(t, stdErr)

	err := Trace(stdErr, http.StatusForbidden, "0123456789abcdef0123456789abcdef", "foo", "bar")
	assertError(t, err)
	assertEqual(t, 1, len(err.(*TracedError).Stack))
	assertContains(t, err.(*TracedError).Stack[0].Function, "TestErrors_Trace")
	assertEqual(t, http.StatusForbidden, err.(*TracedError).StatusCode)
	assertEqual(t, "0123456789abcdef0123456789abcdef", err.(*TracedError).Trace)
	assertEqual(t, "bar", err.(*TracedError).Properties["foo"])

	err = Trace(err, http.StatusNotImplemented, "moo", "baz")
	assertEqual(t, 2, len(err.(*TracedError).Stack))
	assertNotEqual(t, "", err.(*TracedError).String())
	assertEqual(t, http.StatusNotImplemented, err.(*TracedError).StatusCode)
	assertEqual(t, "0123456789abcdef0123456789abcdef", err.(*TracedError).Trace)
	assertEqual(t, "bar", err.(*TracedError).Properties["foo"])
	assertEqual(t, "baz", err.(*TracedError).Properties["moo"])

	err = Trace(err)
	assertEqual(t, 3, len(err.(*TracedError).Stack))
	assertNotEqual(t, "", err.(*TracedError).String())
	assertEqual(t, http.StatusNotImplemented, err.(*TracedError).StatusCode)
	assertEqual(t, "0123456789abcdef0123456789abcdef", err.(*TracedError).Trace)
	assertEqual(t, "bar", err.(*TracedError).Properties["foo"])
	assertEqual(t, "baz", err.(*TracedError).Properties["moo"])

	stdErr = Trace(nil)
	assertNil(t, stdErr)
	assertNil(t, stdErr)
}

func TestErrors_TraceNested(t *testing.T) {
	t.Parallel()

	inner := New("connection refused", http.StatusForbidden, "0123456789abcdef0123456789abcdef", "host", "10.0.0.1")
	wrapped := &url.Error{Op: "Get", URL: "http://10.0.0.1", Err: &net.OpError{Op: "dial", Net: "tcp", Err: inner}}

	// Converting the wrapper inherits the inner error's data and keeps the wrapper's message
	converted := Convert(wrapped)
	assertEqual(t, wrapped.Error(), converted.Error())
	assertEqual(t, http.StatusForbidden, converted.StatusCode)
	assertEqual(t, http.StatusForbidden, StatusCode(wrapped))
	assertEqual(t, "0123456789abcdef0123456789abcdef", converted.Trace)
	assertEqual(t, "10.0.0.1", converted.Properties["host"])
	assertEqual(t, inner.(*TracedError).Stack, converted.Stack)
	assertTrue(t, Is(converted, inner))

	// Tracing the wrapper extends the inner error's stack with the current location
	err := Trace(wrapped)
	tracedErr := Convert(err)
	assertEqual(t, wrapped.Error(), tracedErr.Error())
	assertEqual(t, http.StatusForbidden, tracedErr.StatusCode)
	assertEqual(t, "0123456789abcdef0123456789abcdef", tracedErr.Trace)
	assertEqual(t, "10.0.0.1", tracedErr.Properties["host"])
	assertEqual(t, 2, len(tracedErr.Stack))
	assertEqual(t, inner.(*TracedError).Stack[0], tracedErr.Stack[0])
	assertContains(t, tracedErr.Stack[1].Function, "TestErrors_TraceNested")
	assertEqual(t, 1, len(inner.(*TracedError).Stack))
	assertTrue(t, Is(err, inner))

	// A status code given to Trace overrides the inherited one
	assertEqual(t, http.StatusNotFound, StatusCode(Trace(wrapped, http.StatusNotFound)))

	// Wrapping with fmt.Errorf keeps the status code too
	assertEqual(t, http.StatusForbidden, StatusCode(fmt.Errorf("fetching: %w", inner)))
}

func TestErrors_TraceDoesNotShareStacks(t *testing.T) {
	t.Parallel()

	err := New("base")
	err = Trace(err)
	err = Trace(err) // The stack now has spare capacity
	a := Trace(err)
	b := Trace(err)
	stackA := Convert(a).Stack
	stackB := Convert(b).Stack
	assertEqual(t, 4, len(stackA))
	assertEqual(t, 4, len(stackB))
	assertNotEqual(t, stackA[3].Line, stackB[3].Line)
	assertEqual(t, 3, len(Convert(err).Stack))
}

func TestErrors_Convert(t *testing.T) {
	t.Parallel()

	stdErr := stderrors.New("other standard error")
	assertError(t, stdErr)

	err := Convert(stdErr)
	assertError(t, err)
	assertEqual(t, 0, len(err.Stack))

	stdErr = Trace(err)
	err = Convert(stdErr)
	assertError(t, err)
	assertEqual(t, 1, len(err.Stack))

	err = Convert(nil)
	assertNil(t, err)
}

func TestErrors_JSON(t *testing.T) {
	t.Parallel()

	err := New("error!")

	b, jsonErr := err.(*TracedError).MarshalJSON()
	assertNil(t, jsonErr)

	var unmarshal TracedError
	jsonErr = unmarshal.UnmarshalJSON(b)
	assertNil(t, jsonErr)

	assertEqual(t, err.Error(), unmarshal.Error())
	assertEqual(t, err.(*TracedError).String(), unmarshal.String())
}

func TestErrors_Format(t *testing.T) {
	t.Parallel()

	err := New("my error")

	s := fmt.Sprintf("%s", err)
	assertEqual(t, "my error", s)

	v := fmt.Sprintf("%v", err)
	assertEqual(t, "my error", v)

	vPlus := fmt.Sprintf("%+v", err)
	assertEqual(t, err.(*TracedError).String(), vPlus)
	assertContains(t, vPlus, "errors.TestErrors_Format")
	assertContains(t, vPlus, "errors/errors_test.go:")

	vSharp := fmt.Sprintf("%#v", err)
	assertEqual(t, err.(*TracedError).String(), vSharp)
	assertContains(t, vSharp, "errors.TestErrors_Format")
	assertContains(t, vSharp, "errors/errors_test.go:")
}

func TestErrors_Is(t *testing.T) {
	t.Parallel()

	err := Trace(os.ErrNotExist)
	assertTrue(t, Is(err, os.ErrNotExist))
}

func TestErrors_Join(t *testing.T) {
	t.Parallel()

	e1 := stderrors.New("E1")
	e2 := New("E2", 400)
	e3 := New("E3")
	e3 = Trace(e3)
	e4a := stderrors.New("E4a")
	e4b := stderrors.New("E4b")
	e4 := Join(e4a, e4b)
	j := Join(e1, e2, nil, e3, e4)
	assertTrue(t, Is(j, e1))
	assertTrue(t, Is(j, e2))
	assertTrue(t, Is(j, e3))
	assertTrue(t, Is(j, e4))
	assertTrue(t, Is(j, e4a))
	assertTrue(t, Is(j, e4b))
	jj, ok := j.(*TracedError)
	assertTrue(t, ok)
	if ok {
		assertEqual(t, 1, len(jj.Stack))
		assertEqual(t, 500, jj.StatusCode)
	}

	assertNil(t, Join(nil, nil))

	// A single error is traced as if with Trace, without modifying it
	j1 := Join(e3, nil)
	assertTrue(t, Is(j1, e3))
	assertEqual(t, e3.Error(), j1.Error())
	assertEqual(t, 3, len(Convert(j1).Stack))
	assertEqual(t, 2, len(Convert(e3).Stack))
	assertEqual(t, 400, StatusCode(Join(nil, e2)))
}

func TestErrors_MultiUnwrapNotInherited(t *testing.T) {
	t.Parallel()

	a := New("a", http.StatusNotFound, "0123456789abcdef0123456789abcdef", "k", "a")
	b := New("b", http.StatusServiceUnavailable)

	// No single TracedError speaks for an error that branches into several, so nothing is inherited
	for _, err := range []error{stderrors.Join(a, b), fmt.Errorf("%w; %w", a, b)} {
		assertEqual(t, 500, StatusCode(err))
		converted := Convert(err)
		assertEqual(t, "", converted.Trace)
		assertEqual(t, 0, len(converted.Properties))
		assertEqual(t, 0, len(converted.Stack))
		traced := Convert(Trace(err))
		assertEqual(t, 500, traced.StatusCode)
		assertEqual(t, 1, len(traced.Stack))
		assertEqual(t, 500, StatusCode(New("", err)))
		assertTrue(t, Is(err, a))
		assertTrue(t, Is(err, b))
	}

	// A single-error wrapper around a branching error is not looked into either
	assertEqual(t, 500, StatusCode(fmt.Errorf("wrapped: %w", stderrors.Join(a, b))))
}

func TestErrors_TypedNilInChain(t *testing.T) {
	t.Parallel()

	var nilTracedErr *TracedError
	w := fmt.Errorf("w: %w", error(nilTracedErr))
	assertEqual(t, 500, StatusCode(w))
	assertEqual(t, 0, len(Convert(w).Stack))
	traced := Convert(Trace(w))
	assertEqual(t, 500, traced.StatusCode)
	assertEqual(t, 1, len(traced.Stack))
	assertEqual(t, 500, StatusCode(New("failed", w)))
	assertEqual(t, 500, StatusCode(New("failed: %w", w)))
	assertTrue(t, !Is(Trace(w), os.ErrNotExist))
}

func TestErrors_TypedNil(t *testing.T) {
	t.Parallel()

	var nilTracedErr *TracedError
	var err error = nilTracedErr

	converted := Convert(err)
	assertEqual(t, 500, converted.StatusCode)
	assertEqual(t, "<nil>", converted.Error())
	assertEqual(t, 0, len(converted.Stack))
	assertContains(t, fmt.Sprintf("%+v", converted), "<nil>")
	assertEqual(t, 500, StatusCode(err))

	traced := Trace(err)
	assertEqual(t, "<nil>", traced.Error())
	assertEqual(t, 1, len(Convert(traced).Stack))
	assertNil(t, Unwrap(traced)) // The nil pointer is not exposed to Is and As
	assertTrue(t, !Is(traced, os.ErrNotExist))

	assertEqual(t, "<nil>", Join(err, nil).Error())
	assertEqual(t, "failed: <nil>", New("failed", err).Error())
	caught := CatchPanic(func() error {
		panic(err)
	})
	assertEqual(t, "<nil>", caught.Error())
	assertEqual(t, 500, StatusCode(caught))
}

func TestErrors_NilPointerCause(t *testing.T) {
	t.Parallel()

	var nilURLErr *url.Error
	var err error = nilURLErr

	caught := CatchPanic(func() error {
		panic(err)
	})
	assertEqual(t, "<nil>", caught.Error())
	assertEqual(t, 500, StatusCode(caught))
	assertTrue(t, !Is(caught, os.ErrNotExist))
	var urlErr *url.Error
	assertTrue(t, !As(caught, &urlErr))
	_, jsonErr := json.Marshal(caught)
	assertNil(t, jsonErr)

	assertEqual(t, 500, StatusCode(err))
	assertEqual(t, "<nil>", Convert(err).Error())
	assertEqual(t, "<nil>", Trace(err).Error())
	assertEqual(t, "failed: <nil>", New("failed", err).Error())
	assertEqual(t, 500, StatusCode(fmt.Errorf("wrapped: %w", err)))
}

func TestErrors_SingleErrorJoin(t *testing.T) {
	t.Parallel()

	a := New("a", http.StatusNotFound, "k", "v")
	for _, err := range []error{stderrors.Join(a, nil), stderrors.Join(nil, a), fmt.Errorf("%w %w", a, nil)} {
		assertEqual(t, http.StatusNotFound, StatusCode(err))
		assertEqual(t, "v", Convert(err).Properties["k"])
		assertEqual(t, http.StatusNotFound, StatusCode(Trace(err)))
	}
}

func TestErrors_FormatVerbs(t *testing.T) {
	t.Parallel()

	stdErr := stderrors.New("boom")
	tracedErr := Trace(stdErr)

	// A traced error formats its message like a standard error does
	for _, f := range []string{"%v", "%s", "%q", "%x", "%X", "%10s", "%-10s|", "%.2s", "%+q", "%#q"} {
		assertEqual(t, fmt.Sprintf(f, stdErr), fmt.Sprintf(f, tracedErr))
	}

	// New lets these verbs format an error operand instead of treating them as literal text
	assertEqual(t, "type *errors.errorString", New("type %T", stdErr).Error())
	assertEqual(t, "hex 626f6f6d", New("hex %x", stdErr).Error())
	assertEqual(t, `quoted "boom"`, New("quoted %q", tracedErr).Error())
	assertEqual(t, "type *errors.TracedError", New("type %T", tracedErr).Error())

	// A directive that becomes literal text gives back the args it took, such as a * width
	err := New("size %*d", http.StatusNotFound, stdErr)
	assertEqual(t, "size %*d: boom", err.Error())
	assertEqual(t, http.StatusNotFound, StatusCode(err))
	assertTrue(t, Is(err, stdErr))
}

func TestErrors_UnnamedCauseUnwraps(t *testing.T) {
	t.Parallel()

	cause := stderrors.New("cause")
	err := New("failed", cause)
	assertEqual(t, "failed: cause", err.Error())
	assertEqual(t, cause, Unwrap(Unwrap(err)))

	err = New("", http.StatusBadRequest, cause)
	assertEqual(t, "bad request: cause", err.Error())
	assertEqual(t, cause, Unwrap(Unwrap(err)))

	// With more than one cause, each remains reachable through Is
	other := stderrors.New("other")
	err = New("failed", cause, other)
	assertEqual(t, "failed: cause: other", err.Error())
	assertTrue(t, Is(err, cause))
	assertTrue(t, Is(err, other))
}

func TestErrors_StackFramesNotShared(t *testing.T) {
	t.Parallel()

	original := New("original")
	originalFile := original.(*TracedError).Stack[0].File
	for _, derived := range []*TracedError{Convert(Trace(original)), Convert(fmt.Errorf("wrapped: %w", original))} {
		derived.Stack[0].File = "redacted"
		assertEqual(t, originalFile, original.(*TracedError).Stack[0].File)
	}
}

func TestErrors_NullStackFrame(t *testing.T) {
	t.Parallel()

	var tracedErr TracedError
	assertNil(t, json.Unmarshal([]byte(`{"error":"x","stack":[null,{"func":"f","file":"f.go","line":1},null]}`), &tracedErr))
	assertEqual(t, 1, len(tracedErr.Stack))
	assertContains(t, tracedErr.String(), "f.go:1")

	// A null frame in a stack built some other way is skipped when rendering
	tracedErr.Stack = append(tracedErr.Stack, nil)
	assertContains(t, tracedErr.String(), "f.go:1")
}

func TestErrors_NewInheritsFromPatternWrap(t *testing.T) {
	t.Parallel()

	inner := New("x", http.StatusNotFound, "0123456789abcdef0123456789abcdef", "k", "v")
	err := New("fetch: %w", inner)
	tracedErr := Convert(err)
	assertEqual(t, "fetch: x", err.Error())
	assertEqual(t, http.StatusNotFound, tracedErr.StatusCode)
	assertEqual(t, "0123456789abcdef0123456789abcdef", tracedErr.Trace)
	assertEqual(t, "v", tracedErr.Properties["k"])
	assertEqual(t, 2, len(tracedErr.Stack))
	assertEqual(t, inner.(*TracedError).Stack[0], tracedErr.Stack[0])
	assertEqual(t, 1, len(inner.(*TracedError).Stack))
	assertTrue(t, Is(err, inner))
	assertEqual(t, StatusCode(fmt.Errorf("fetch: %w", inner)), StatusCode(err))

	// An explicit status code takes precedence
	assertEqual(t, http.StatusBadGateway, StatusCode(New("fetch: %w", inner, http.StatusBadGateway)))

	// %v prints the error without wrapping it, so nothing is inherited
	err = New("fetch: %v", inner)
	assertEqual(t, "fetch: x", err.Error())
	assertEqual(t, 500, StatusCode(err))
	assertTrue(t, !Is(err, inner))
}

func TestErrors_NewInheritsFromFirstError(t *testing.T) {
	t.Parallel()

	a := New("a", http.StatusNotFound, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "k", "a", "onlyA", 1)
	b := New("b", http.StatusServiceUnavailable, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "k", "b", "onlyB", 2)
	for _, err := range []error{New("x", a, b), New("x: %w", a, b), New("%w: %w", a, b)} {
		tracedErr := Convert(err)
		assertEqual(t, http.StatusNotFound, tracedErr.StatusCode)
		assertEqual(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", tracedErr.Trace)
		assertEqual(t, 2, len(tracedErr.Stack))
		assertEqual(t, a.(*TracedError).Stack[0], tracedErr.Stack[0])
		assertEqual(t, "a", tracedErr.Properties["k"])
		assertEqual(t, 1, tracedErr.Properties["onlyA"])
		assertEqual(t, 2, tracedErr.Properties["onlyB"])
	}

	// Explicit arguments take precedence regardless of their order
	tracedErr := Convert(New("x", "k", "explicit", http.StatusConflict, "cccccccccccccccccccccccccccccccc", a))
	assertEqual(t, http.StatusConflict, tracedErr.StatusCode)
	assertEqual(t, "cccccccccccccccccccccccccccccccc", tracedErr.Trace)
	assertEqual(t, "explicit", tracedErr.Properties["k"])
	assertEqual(t, 1, tracedErr.Properties["onlyA"])
}

func TestErrors_LiteralPercent(t *testing.T) {
	t.Parallel()

	inner := New("x", http.StatusNotFound)

	// A % sign that cannot format the error is literal text and the error is wrapped
	err := New("100% done", inner)
	assertEqual(t, "100% done: x", err.Error())
	assertTrue(t, Is(err, inner))
	assertEqual(t, http.StatusNotFound, StatusCode(err))

	err = New("%d%% done, 100% left", 5, inner)
	assertEqual(t, "5% done, 100% left: x", err.Error())
	assertTrue(t, Is(err, inner))

	// A trailing % sign is literal text
	assertEqual(t, "progress: 100%", New("progress: 100%").Error())
	assertEqual(t, "progress: 100%: x", New("progress: 100%", inner).Error())

	// Verbs that can print an error still consume it
	assertEqual(t, "failed: x", New("failed: %s", inner).Error())

	// Argument indexes, widths and precisions are parsed as fmt does
	err = New("%[2]s %[1]s", "a", "b", http.StatusBadRequest)
	assertEqual(t, "b a", err.Error())
	assertEqual(t, http.StatusBadRequest, StatusCode(err))
	err = New("[%*d] [%-5.2f]", 4, 7, 3.14159, http.StatusBadRequest)
	assertEqual(t, "[   7] [3.14 ]", err.Error())
	assertEqual(t, http.StatusBadRequest, StatusCode(err))
}

func TestErrors_SharedErrorNotModified(t *testing.T) {
	t.Parallel()

	sentinel := New("not found", http.StatusNotFound)
	stackLen := len(sentinel.(*TracedError).Stack)

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			j := Join(sentinel, nil)
			assertTrue(t, Is(j, sentinel))
			assertEqual(t, http.StatusNotFound, StatusCode(j))
			assertEqual(t, stackLen+1, len(Convert(j).Stack))

			caught := CatchPanic(func() error {
				panic(sentinel)
			})
			assertTrue(t, Is(caught, sentinel))
			assertEqual(t, http.StatusNotFound, StatusCode(caught))
			assertTrue(t, len(Convert(caught).Stack) > stackLen)
		}()
	}
	wg.Wait()
	assertEqual(t, stackLen, len(sentinel.(*TracedError).Stack))

	// Converting an error with no status code does not write the default into it
	noStatus := &TracedError{Err: stderrors.New("no status")}
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			assertEqual(t, 500, StatusCode(noStatus))
			assertTrue(t, Is(Convert(noStatus), noStatus))
		}()
	}
	wg.Wait()
	assertEqual(t, 0, noStatus.StatusCode)
}

func TestErrors_String(t *testing.T) {
	t.Parallel()

	err := New("oops!", 400, "key", "value")
	err = Trace(err)
	s := err.(*TracedError).String()
	assertContains(t, s, "oops!")
	assertContains(t, s, "400")
	assertContains(t, s, "/errors/errors_test.go:")
	assertContains(t, s, "key")
	assertContains(t, s, "value")
	firstDash := strings.Index(s, "-")
	assertTrue(t, firstDash > 0)
	secondDash := strings.Index(s[firstDash+1:], "-")
	assertTrue(t, secondDash > 0)
}

func TestErrors_Unwrap(t *testing.T) {
	t.Parallel()

	stdErr := stderrors.New("oops")
	err := Trace(stdErr)
	assertEqual(t, stdErr, Unwrap(err))

	err = New("", stdErr)
	assertEqual(t, stdErr, Unwrap(err))

	err = New("failed: %w", stdErr)
	assertEqual(t, stdErr, Unwrap(Unwrap(err)))
	assertTrue(t, Is(err, stdErr))

	inlineErr := stderrors.New("inline")
	arg1Err := stderrors.New("arg1")
	arg2Err := stderrors.New("arg2")
	err = New("failed: %w", inlineErr, arg1Err, "id", 123, arg2Err)
	assertEqual(t, "failed: inline: arg1: arg2", err.Error())
	assertTrue(t, Is(err, inlineErr))
	assertTrue(t, Is(err, arg1Err))
	assertTrue(t, Is(err, arg2Err))
}

func TestErrors_CatchPanic(t *testing.T) {
	t.Parallel()

	// String
	err := CatchPanic(func() error {
		panic("message")
	})
	assertError(t, err)
	assertEqual(t, "message", err.Error())

	// Error
	err = CatchPanic(func() error {
		panic(New("panic"))
	})
	assertError(t, err)
	assertEqual(t, "panic", err.Error())

	// Number
	err = CatchPanic(func() error {
		panic(5)
	})
	assertError(t, err)
	assertEqual(t, "5", err.Error())

	// Division by zero
	err = CatchPanic(func() error {
		j := 1
		j--
		i := 5 / j
		i++
		return nil
	})
	assertError(t, err)
	assertEqual(t, "runtime error: integer divide by zero", err.Error())

	// Nil map
	err = CatchPanic(func() error {
		x := map[int]int{}
		if true {
			x = nil
		}
		x[5] = 6
		return nil
	})
	assertError(t, err)
	assertEqual(t, "assignment to entry in nil map", err.Error())

	// Standard error
	err = CatchPanic(func() error {
		return New("standard")
	})
	assertError(t, err)
	assertEqual(t, "standard", err.Error())
}

func TestErrors_AnonymousProperties(t *testing.T) {
	t.Parallel()

	// Errors
	base := stderrors.New("base")
	err := New("failed", base)
	assertEqual(t, "failed: base", err.Error())
	assertTrue(t, Is(err, base))

	// Status code
	err = New("failed", 409)
	assertEqual(t, 409, Convert(err).StatusCode)

	// Trace ID
	err = New("failed", "0123456789abcdef0123456789abcdef")
	assertEqual(t, "0123456789abcdef0123456789abcdef", Convert(err).Trace)
}
