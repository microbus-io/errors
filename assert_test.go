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
	"strings"
	"testing"
)

func assertError(t *testing.T, err error) {
	if err == nil {
		t.Errorf("got nil, want error")
	}
}

func assertEqual(t *testing.T, want any, got any) {
	if !reflect.DeepEqual(want, got) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func assertNotEqual(t *testing.T, want any, got any) {
	if reflect.DeepEqual(want, got) {
		t.Errorf("got %v unexpectedly", got)
	}
}

func assertContains(t *testing.T, whole string, part string) {
	if !strings.Contains(whole, part) {
		t.Errorf("got %v, want to contain %v", whole, part)
	}
}

func assertTrue(t *testing.T, cond bool) {
	if !cond {
		t.Error("got false, want true")
	}
}

func assertNil(t *testing.T, obj any) {
	isNil := func(obj any) bool {
		defer func() { recover() }()
		return obj == nil || reflect.ValueOf(obj).IsNil()
	}
	if !isNil(obj) {
		t.Errorf("got %v, want nil", obj)
	}
}
