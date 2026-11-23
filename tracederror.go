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
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

const zeroTrace = "00000000000000000000000000000000"

// Ensure interfaces
var (
	_ = error(&TracedError{})
	_ = fmt.Stringer(&TracedError{})
	_ = fmt.Formatter(&TracedError{})
	_ = json.Marshaler(&TracedError{})
	_ = json.Unmarshaler(&TracedError{})
)

// TracedError is a standard Go error augmented with a stack trace, status code and property bag.
type TracedError struct {
	Err        error
	Stack      []*StackFrame
	StatusCode int
	Trace      string
	Properties map[string]any
}

/*
New creates a new error, with a static or formatted message, optionally wrapping another error, attaching a status code or attaching properties.

In the simplest case, the pattern is a static string.

	New("network timeout")

If the pattern contains % signs, the next appropriate number of arguments are used to format the message of the new error as if with fmt.Errorf.

	New("failed to parse '%s' for user %d", dateStr, userID)

Any additional arguments are treated like slog name=value pairs and added to the error's property bag.
Properties are not part of the error's message and can be retrieved up the call stack in a structured way.

	New("failed to execute '%s'", cmd,
		"exitCode", exitCode,
		"os", os,
	)

Three notable properties do not require a name: errors, integers and 32-character long hex string.

	New("failed to parse form",
		err,
		http.StatusBadRequest,
		"ba0da7b3d3150f20702229c4521b58e9",
		"path", r.URL.Path,
	)

An unnamed error is interpreted to be the original source of the error. The new error is created to wrap the original error as if with

	fmt.Errorf(errorMessage+": %w", originalError)

If an error wrapped by the new error, either as an unnamed argument or with %w in the pattern, is a TracedError or wraps
one further down its chain, the new error inherits from it:

  - The first such TracedError provides the status code, trace ID and stack trace, the last of which is extended with
    the current location.
  - The properties of all such TracedErrors are merged, with those found first taking precedence.
  - A status code, trace ID or property given explicitly takes precedence over an inherited one.
  - Only errors that wrap a single error are looked into. An error that branches into several errors, as with
    errors.Join or fmt.Errorf with more than one %w, has no single TracedError to inherit from.

An unnamed integer is interpreted to be an HTTP status code to associate with the error. If the pattern is empty, the status text is set by default.

An unnamed 32-character long hex string is interpreted to be a trace ID.

A % sign that would format an error argument with a verb that cannot format an error, which is any verb other than %v,
%s, %q, %x, %X, %T, %p or %w, is treated as literal text, as is a % sign at the end of the pattern. An example is the
"% d" in "100% done". The error is then wrapped instead. To be safe with other arguments, write a literal % sign as %%.
*/
func New(pattern string, args ...any) error {
	err := &TracedError{}
	var wrapped []error // The errors that the new error wraps, in order
	i := 0
	if pattern != "" {
		// Important: Trace expects that an empty pattern will not wrap followup error objects
		var format string
		format, i = formatArgs(pattern, args)
		err.Err = fmt.Errorf(format, args[:i]...)
		switch x := err.Err.(type) {
		case interface{ Unwrap() error }:
			wrapped = append(wrapped, x.Unwrap())
		case interface{ Unwrap() []error }:
			wrapped = append(wrapped, x.Unwrap()...)
		}
	}
	for i < len(args) {
		if err.Properties == nil {
			err.Properties = make(map[string]any, len(args)-i)
		}
		switch k := args[i].(type) {
		case int:
			err.StatusCode = k
			if err.Err == nil {
				err.Err = stderrors.New(statusText[k])
			}
			i++
		case error:
			if err.Err == nil {
				// Important: Trace expects that an empty pattern will not wrap followup error objects
				err.Err = k
			} else if len(wrapped) == 0 {
				// The message wraps no error, so wrap only this one, keeping it reachable through Unwrap
				err.Err = fmt.Errorf("%s: %w", err.Err, k)
			} else {
				err.Err = fmt.Errorf("%w: %w", err.Err, k)
			}
			wrapped = append(wrapped, k)
			i++
		case string:
			isHex := func(s string) bool {
				for i := range s {
					c := s[i]
					if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
						return false
					}
				}
				return true
			}
			if len(k) == 32 && isHex(k) {
				err.Trace = k
				i++
			} else if i < len(args)-1 {
				err.Properties[k] = args[i+1]
				i += 2
			} else {
				err.Properties[k] = ""
				i++
			}
		default:
			err.Properties["!BADKEY"] = k
			i++
		}
	}
	// Inherit from the wrapped TracedErrors only now, so that explicit arguments take precedence regardless of their order
	inherited := false
	for _, w := range wrapped {
		tracedErr := findTraced(w)
		if tracedErr == nil {
			continue
		}
		if !inherited {
			inherited = true
			if err.StatusCode == 0 {
				err.StatusCode = tracedErr.StatusCode
			}
			if err.Trace == "" || err.Trace == zeroTrace {
				err.Trace = tracedErr.Trace
			}
			err.Stack = cloneStack(tracedErr.Stack)
		}
		for k, v := range tracedErr.Properties {
			if _, ok := err.Properties[k]; !ok {
				if err.Properties == nil {
					err.Properties = make(map[string]any, len(tracedErr.Properties))
				}
				err.Properties[k] = v
			}
		}
	}
	if err.Err == nil {
		err.Err = stderrors.New("unspecified error")
	}
	if err.StatusCode == 0 {
		err.StatusCode = 500
	}
	return traceCaller(err)
}

// formatArgs returns the format to pass to fmt.Errorf for the pattern, and how many of the leading args it consumes.
// It parses the pattern's verbs as fmt does. A verb that would consume an error but cannot format one, which is any verb
// other than %v, %s, %q, %x, %X, %T, %p or %w, is taken to be literal text rather than a verb, such as the "% d" in
// "100% done". So is a % sign at the end of the pattern. That % sign and all those after it are escaped in the returned
// format, and neither the args of that directive nor any after it are consumed.
func formatArgs(pattern string, args []any) (format string, consumed int) {
	argNum := 0
	// operand consumes the arg at argNum for the verb, or reports false if the verb cannot print it
	operand := func(verb rune) bool {
		if argNum >= len(args) {
			return true // fmt reports the missing operand
		}
		if _, ok := args[argNum].(error); ok && !strings.ContainsRune("vsqxXTpw", verb) {
			return false
		}
		argNum++
		consumed = max(consumed, argNum)
		return true
	}
	// argIndex parses an explicit argument index such as [2] at position i and returns the position after it
	argIndex := func(i int) int {
		if i >= len(pattern) || pattern[i] != '[' {
			return i
		}
		end := strings.IndexByte(pattern[i:], ']')
		if end < 0 {
			return i
		}
		if n, err := strconv.Atoi(pattern[i+1 : i+end]); err == nil && n >= 1 && n <= len(args) {
			argNum = n - 1
		}
		return i + end + 1
	}
	digits := func(i int) int {
		for i < len(pattern) && pattern[i] >= '0' && pattern[i] <= '9' {
			i++
		}
		return i
	}
	literal := func(start int) string {
		var b strings.Builder
		b.WriteString(pattern[:start])
		for i := start; i < len(pattern); i++ {
			if pattern[i] == '%' {
				b.WriteString("%%")
				if i+1 < len(pattern) && pattern[i+1] == '%' {
					i++ // Already escaped
				}
			} else {
				b.WriteByte(pattern[i])
			}
		}
		return b.String()
	}
	for i := 0; i < len(pattern); {
		if pattern[i] != '%' {
			i++
			continue
		}
		start, consumedBefore := i, consumed
		i++
		for i < len(pattern) && strings.IndexByte("#0+- ", pattern[i]) >= 0 {
			i++ // Flags
		}
		i = argIndex(i)
		if i < len(pattern) && pattern[i] == '*' {
			if !operand('*') {
				return literal(start), consumedBefore
			}
			i++
		} else {
			i = digits(i) // Width
		}
		if i < len(pattern) && pattern[i] == '.' {
			i = argIndex(i + 1)
			if i < len(pattern) && pattern[i] == '*' {
				if !operand('*') {
					return literal(start), consumedBefore
				}
				i++
			} else {
				i = digits(i) // Precision
			}
		}
		i = argIndex(i)
		if i >= len(pattern) {
			return literal(start), consumedBefore
		}
		verb, size := utf8.DecodeRuneInString(pattern[i:])
		i += size
		if verb != '%' && !operand(verb) {
			return literal(start), consumedBefore
		}
	}
	return pattern, consumed
}

// Error returns the error string.
// A nil *TracedError, or one that wraps a nil pointer such as a nil *url.Error, returns "<nil>", as fmt prints it.
func (e *TracedError) Error() string {
	if e == nil || isNilPointer(e.Err) {
		return "<nil>"
	}
	return e.Err.Error()
}

// Unwrap returns the underlying error.
// A nil *TracedError, or one that wraps a nil pointer such as a nil *url.Error, returns nil. This ends the chain for Is
// and As, which would otherwise call the nil pointer's methods.
func (e *TracedError) Unwrap() error {
	if e == nil || isNilPointer(e.Err) {
		return nil
	}
	return e.Err
}

// String returns a human-friendly representation of the traced error.
func (e *TracedError) String() string {
	var b strings.Builder
	b.WriteString(e.Error())
	if e.StatusCode != 0 && e.StatusCode != 500 {
		b.WriteString("\nstatusCode=")
		b.WriteString(fmt.Sprintf("%d", e.StatusCode))
	}
	if e.Trace != "" && e.Trace != zeroTrace {
		b.WriteString("\ntrace=")
		b.WriteString(e.Trace)
	}
	for k, v := range e.Properties {
		b.WriteString("\n")
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(fmt.Sprintf("%v", v))
	}
	if len(e.Stack) > 0 {
		b.WriteString("\n")
	}
	for _, stackFrame := range e.Stack {
		if stackFrame == nil {
			continue
		}
		b.WriteString("\n")
		b.WriteString(stackFrame.String())
	}
	return b.String()
}

// MarshalJSON marshals the error to JSON.
func (e *TracedError) MarshalJSON() ([]byte, error) {
	m := map[string]any{}
	if len(e.Properties) > 0 {
		maps.Copy(m, e.Properties)
	}
	m["error"] = e.Error()
	if e.StatusCode != 0 {
		m["statusCode"] = e.StatusCode
	} else {
		delete(m, "statusCode")
	}
	if e.Stack != nil {
		m["stack"] = e.Stack
	} else {
		delete(m, "stack")
	}
	if e.Trace != "" && e.Trace != zeroTrace {
		m["trace"] = e.Trace
	} else {
		delete(m, "trace")
	}
	return json.Marshal(m)
}

// UnmarshalJSON unmarshals the error from JSON.
// Neither the type of the error nor any errors it wraps can be restored.
func (e *TracedError) UnmarshalJSON(data []byte) error {
	var j StreamedError
	err := json.Unmarshal(data, &j)
	if err != nil {
		return err
	}
	e.Err = stderrors.New(j.Error)
	e.Stack = slices.DeleteFunc(j.Stack, func(frame *StackFrame) bool { return frame == nil }) // Drop null frames
	e.StatusCode = j.StatusCode
	e.Trace = j.Trace

	var m map[string]any
	err = json.Unmarshal(data, &m)
	if err != nil {
		return err
	}
	delete(m, "error")
	delete(m, "statusCode")
	delete(m, "stack")
	delete(m, "trace")
	if len(m) > 0 {
		e.Properties = m
	} else {
		e.Properties = nil
	}
	return nil
}

// Format the error based on the verb and flag.
// %+v and %#v print the stack trace, status code and properties. %v, %s, %q, %x and %X format the error message as
// they would a standard error.
func (e *TracedError) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		if s.Flag('+') || s.Flag('#') {
			io.WriteString(s, e.String())
			return
		}
		fallthrough
	case 's', 'q', 'x', 'X':
		fmt.Fprintf(s, fmt.FormatString(s, verb), e.Error())
	}
}

// StreamedError is the schema used to marshal and unmarshal the traced error.
type StreamedError struct {
	Error      string        `json:"error" jsonschema:"example=message"`
	StatusCode int           `json:"statusCode,omitzero"`
	Trace      string        `json:"trace,omitzero"`
	Stack      []*StackFrame `json:"stack,omitzero"`
}

// StackFrame is a single stack location.
type StackFrame struct {
	Function string `json:"func"`
	File     string `json:"file"`
	Line     int    `json:"line"`
}

// String returns a string representation of the stack frame.
func (t *StackFrame) String() string {
	return fmt.Sprintf("- %s\n  %s:%d", t.Function, t.File, t.Line)
}
