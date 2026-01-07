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
	"runtime"
	"strings"
)

// traceCaller appends the stack location of the caller to the error's stack trace.
func traceCaller(err error) error {
	if err == nil {
		return nil
	}
	level := 1
	tracedErr := Convert(err)
	for {
		file, function, line, ok := runtimeTrace(level)
		if !ok {
			return tracedErr
		}
		if strings.HasPrefix(function, "runtime.") || (strings.HasPrefix(function, "errors.") && !strings.HasPrefix(function, "errors.Test")) {
			level++
			continue
		}
		tracedErr.Stack = append(tracedErr.Stack, &StackFrame{
			File:     file,
			Function: function,
			Line:     line,
		})
		return tracedErr
	}
}

// traceFull appends the full stack to the error's stack trace, starting at the indicated level.
// Level 0 captures the location of the caller.
func traceFull(err error, level int) error {
	if err == nil {
		return nil
	}
	if level < 0 {
		level = 0
	}
	tracedErr := Convert(err)

	levels := level - 1
	for {
		levels++
		file, function, line, ok := runtimeTrace(1 + levels)
		if !ok {
			break
		}
		if function == "errors.CatchPanic" {
			break
		}
		if strings.HasPrefix(function, "runtime.") || (strings.HasPrefix(function, "errors.") && !strings.HasPrefix(function, "errors.Test")) {
			continue
		}
		tracedErr.Stack = append(tracedErr.Stack, &StackFrame{
			File:     file,
			Function: function,
			Line:     line,
		})
	}
	return tracedErr
}

// runtimeTrace traces back by the amount of levels to retrieve the runtime information used for tracing.
func runtimeTrace(levels int) (file string, function string, line int, ok bool) {
	pc, file, line, ok := runtime.Caller(levels + 1)
	if !ok {
		return "", "", 0, false
	}
	function = "?"
	runtimeFunc := runtime.FuncForPC(pc)
	if runtimeFunc != nil {
		function = runtimeFunc.Name()
		p := strings.LastIndex(function, "/")
		if p >= 0 {
			function = function[p+1:]
		}
	}
	return file, function, line, ok
}
