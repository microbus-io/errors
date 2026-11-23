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
	"reflect"
	"runtime"
	"strings"
)

// pkgPath is the import path of this package, such as github.com/microbus-io/errors.
var pkgPath = reflect.TypeFor[TracedError]().PkgPath()

// isInternalFrame reports whether a function, given by its full name, belongs to the runtime or to this package, other
// than to its tests. Its frames are left out of stack traces.
// The full name is required so that functions of other packages that happen to be named errors are not left out too.
func isInternalFrame(function string) bool {
	if strings.HasPrefix(function, "runtime.") {
		return true
	}
	rest, ok := strings.CutPrefix(function, pkgPath+".")
	return ok && !strings.HasPrefix(rest, "Test")
}

// shortFunctionName removes the package path from a function's full name, leaving the package name.
func shortFunctionName(function string) string {
	if p := strings.LastIndex(function, "/"); p >= 0 {
		return function[p+1:]
	}
	return function
}

// cloneStack returns a copy of the stack trace whose frames can be modified without affecting the original.
func cloneStack(stack []*StackFrame) []*StackFrame {
	if stack == nil {
		return nil
	}
	frames := make([]StackFrame, len(stack)) // One allocation for all frames
	clone := make([]*StackFrame, len(stack))
	for i, frame := range stack {
		if frame != nil {
			frames[i] = *frame
			clone[i] = &frames[i]
		}
	}
	return clone
}

// traceCaller appends the stack location of the caller to the error's stack trace.
// The error is modified in place, so the caller must own it rather than share it.
func traceCaller(tracedErr *TracedError) error {
	if tracedErr == nil {
		return nil
	}
	level := 1
	for {
		file, function, line, ok := runtimeTrace(level)
		if !ok {
			return tracedErr
		}
		if isInternalFrame(function) {
			level++
			continue
		}
		tracedErr.Stack = append(tracedErr.Stack, &StackFrame{
			File:     file,
			Function: shortFunctionName(function),
			Line:     line,
		})
		return tracedErr
	}
}

// traceFull appends the full stack to the error's stack trace, starting at the indicated level.
// Level 0 captures the location of the caller.
// The error is modified in place, so the caller must own it rather than share it.
func traceFull(tracedErr *TracedError, level int) error {
	if tracedErr == nil {
		return nil
	}
	if level < 0 {
		level = 0
	}

	levels := level - 1
	for {
		levels++
		file, function, line, ok := runtimeTrace(1 + levels)
		if !ok {
			break
		}
		if function == pkgPath+".CatchPanic" {
			break
		}
		if isInternalFrame(function) {
			continue
		}
		tracedErr.Stack = append(tracedErr.Stack, &StackFrame{
			File:     file,
			Function: shortFunctionName(function),
			Line:     line,
		})
	}
	return tracedErr
}

// runtimeTrace traces back by the amount of levels to retrieve the runtime information used for tracing.
// The function is identified by its full name, including its package path.
func runtimeTrace(levels int) (file string, function string, line int, ok bool) {
	pc, file, line, ok := runtime.Caller(levels + 1)
	if !ok {
		return "", "", 0, false
	}
	function = "?"
	runtimeFunc := runtime.FuncForPC(pc)
	if runtimeFunc != nil {
		function = runtimeFunc.Name()
	}
	return file, function, line, ok
}
