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
	"testing"
)

func TestErrors_RuntimeTrace(t *testing.T) {
	t.Parallel()

	file, function, line1, _ := runtimeTrace(0)
	_, _, line2, _ := runtimeTrace(0)
	assertContains(t, file, "trace_test.go")
	assertEqual(t, "errors.TestErrors_RuntimeTrace", function)
	assertEqual(t, line1+1, line2)
}

func TestErrors_TraceFull(t *testing.T) {
	t.Parallel()

	stdErr := stderrors.New("Oops")
	err := Trace(stdErr)
	errFull := traceFull(stdErr, 0)

	tracedErr := Convert(err)
	assertEqual(t, 1, len(tracedErr.Stack))

	assertEqual(t, 2, len(errFull.(*TracedError).Stack))
	assertEqual(t, "errors.TestErrors_TraceFull", errFull.(*TracedError).Stack[0].Function)
	assertEqual(t, "testing.tRunner", errFull.(*TracedError).Stack[1].Function)
}

func TestErrors_TraceCaller(t *testing.T) {
	t.Parallel()

	err := stderrors.New("hello")
	err0 := traceCaller(err)
	assertEqual(t, "errors.TestErrors_TraceCaller", Convert(err0).Stack[0].Function)
}
