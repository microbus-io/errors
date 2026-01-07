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
	"net/http"
	"os"
	"strings"
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
	assertEqual(t, e3, Join(e3, nil))
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
