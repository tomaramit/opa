// Copyright 2016 The OPA Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package eval

import (
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"testing"

	"github.com/open-policy-agent/opa/opalog"
)

func TestLoadFromJSONFiles(t *testing.T) {
	tmp1, err := ioutil.TempFile("", "file1")
	if err != nil {
		panic(err)
	}

	defer os.Remove(tmp1.Name())

	tmp2, err := ioutil.TempFile("", "file2")
	if err != nil {
		panic(err)
	}

	defer os.Remove(tmp2.Name())

	doc1 := `{"foo": "bar"}`
	doc2 := `{"bar": "baz"}`

	if _, err := tmp1.Write([]byte(doc1)); err != nil {
		panic(err)
	}

	if _, err := tmp2.Write([]byte(doc2)); err != nil {
		panic(err)
	}

	if err := tmp1.Close(); err != nil {
		panic(err)
	}
	if err := tmp2.Close(); err != nil {
		panic(err)
	}

	store, err := NewStorageFromJSONFiles([]string{tmp1.Name(), tmp2.Name()})

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	exp, err := store.Get(path("foo"))
	if Compare(exp, "bar") != 0 || err != nil {
		t.Errorf("Expected %v but got %v (err: %v)", "bar", exp, err)
	}

	exp, err = store.Get(path("bar"))
	if Compare(exp, "baz") != 0 || err != nil {
		t.Errorf("Expected %v but got %v (err: %v)", "baz", exp, err)
	}
}

func TestStorageGet(t *testing.T) {

	data := loadSmallTestData()

	var tests = []struct {
		ref      string
		expected interface{}
	}{
		{"a[0]", float64(1)},
		{"a[3]", float64(4)},
		{"b.v1", "hello"},
		{"b.v2", "goodbye"},
		{"c[0].x[1]", false},
		{"c[0].y[0]", nil},
		{"c[0].y[1]", 3.14159},
		{"d.e[1]", "baz"},
		{"d.e", []interface{}{"bar", "baz"}},
		{"c[0].z", map[string]interface{}{"p": true, "q": false}},
		{"d[100]", notFoundError(path("d[100]"), objectKeyTypeMsg(float64(100)))},
		{"dead.beef", notFoundError(path("dead.beef"), doesNotExistMsg)},
		{"a.str", notFoundError(path("a.str"), arrayIndexTypeMsg("str"))},
		{"a[100]", notFoundError(path("a[100]"), outOfRangeMsg)},
		{"a[-1]", notFoundError(path("a[-1]"), outOfRangeMsg)},
		{"b.vdeadbeef", notFoundError(path("b.vdeadbeef"), doesNotExistMsg)},
	}

	store := NewStorageFromJSONObject(data)

	for idx, tc := range tests {
		ref := parseRef(tc.ref)
		path, err := ref.Underlying()
		if err != nil {
			panic(err)
		}
		result, err := store.Get(path)
		switch e := tc.expected.(type) {
		case error:
			if err == nil {
				t.Errorf("Test case %d: expected error for %v but got %v", idx+1, ref, result)
			} else if !reflect.DeepEqual(err, tc.expected) {
				t.Errorf("Test case %d: unexpected error for %v: %v, expected: %v", idx+1, ref, err, e)
			}
		default:
			if err != nil {
				t.Errorf("Test case %d: expected success for %v but got %v", idx+1, ref, err)
			}
			if !reflect.DeepEqual(result, tc.expected) {
				t.Errorf("Test case %d: expected %f but got %f", idx+1, tc.expected, result)
			}
		}
	}

}

func TestStoragePatch(t *testing.T) {

	tests := []struct {
		note        string
		op          string
		path        interface{}
		value       string
		expected    error
		getPath     interface{}
		getExpected interface{}
	}{
		{"add root", "add", path("newroot"), `{"a": [[1]]}`, nil, path("newroot"), `{"a": [[1]]}`},
		{"add root/arr", "add", path("a[1]"), `"x"`, nil, path("a"), `[1,"x",2,3,4]`},
		{"add arr/arr", "add", path("h[1][2]"), `"x"`, nil, path("h"), `[[1,2,3], [2,3,"x",4]]`},
		{"add obj/arr", "add", path("d.e[1]"), `"x"`, nil, path("d"), `{"e": ["bar", "x", "baz"]}`},
		{"add obj", "add", path("b.vNew"), `"x"`, nil, path("b"), `{"v1": "hello", "v2": "goodbye", "vNew": "x"}`},

		{"append root/arr", "add", path(`a["-"]`), `"x"`, nil, path("a"), `[1,2,3,4,"x"]`},
		{"append obj/arr", "add", path(`c[0].x["-"]`), `"x"`, nil, path("c[0].x"), `[true,false,"foo","x"]`},
		{"append arr/arr", "add", path(`h[0]["-"]`), `"x"`, nil, path(`h[0][3]`), `"x"`},

		{"remove root", "remove", path("a"), "", nil, path("a"), notFoundError(path("a"), doesNotExistMsg)},
		{"remove root/arr", "remove", path("a[1]"), "", nil, path("a"), "[1,3,4]"},
		{"remove obj/arr", "remove", path("c[0].x[1]"), "", nil, path("c[0].x"), `[true,"foo"]`},
		{"remove arr/arr", "remove", path("h[0][1]"), "", nil, path("h[0]"), "[1,3]"},
		{"remove obj", "remove", path("b.v2"), "", nil, path("b"), `{"v1": "hello"}`},

		{"replace root", "replace", path("a"), "1", nil, path("a"), "1"},
		{"replace obj", "replace", path("b.v1"), "1", nil, path("b"), `{"v1": 1, "v2": "goodbye"}`},
		{"replace array", "replace", path("a[1]"), "999", nil, path("a"), "[1,999,3,4]"},

		{"err: empty path", "add", []interface{}{}, "", notFoundError([]interface{}{}, nonEmptyMsg), nil, nil},
		{"err: non-string head", "add", []interface{}{float64(1)}, "", notFoundError([]interface{}{float64(1)}, stringHeadMsg), nil, nil},
		{"err: add arr (non-integer)", "add", path("a.foo"), "1", notFoundError(path("a.foo"), arrayIndexTypeMsg("xxx")), nil, nil},
		{"err: add arr (non-integer)", "add", path("a[3.14]"), "1", notFoundError(path("a[3.14]"), arrayIndexTypeMsg(3.14)), nil, nil},
		{"err: add arr (out of range)", "add", path("a[5]"), "1", notFoundError(path("a[5]"), outOfRangeMsg), nil, nil},
		{"err: add arr (out of range)", "add", path("a[-1]"), "1", notFoundError(path("a[-1]"), outOfRangeMsg), nil, nil},
		{"err: add arr (missing root)", "add", path("dead.beef[0]"), "1", notFoundError(path("dead.beef"), doesNotExistMsg), nil, nil},
		{"err: add obj (non-string)", "add", path("b[100]"), "1", notFoundError(path("b[100]"), objectKeyTypeMsg(float64(100))), nil, nil},
		{"err: add non-coll", "add", path("a[1][2]"), "1", notFoundError(path("a[1][2]"), nonCollectionMsg(float64(1))), nil, nil},
		{"err: append (missing)", "add", path(`dead.beef["-"]`), "1", notFoundError(path("dead"), doesNotExistMsg), nil, nil},
		{"err: append obj/arr", "add", path(`c[0].deadbeef["-"]`), `"x"`, notFoundError(path("c[0].deadbeef"), doesNotExistMsg), nil, nil},
		{"err: append arr/arr (out of range)", "add", path(`h[9999]["-"]`), `"x"`, notFoundError(path("h[9999]"), outOfRangeMsg), nil, nil},
		{"err: append arr/arr (non-array)", "add", path(`b.v1["-"]`), "1", notFoundError(path("b.v1"), nonArrayMsg("v1")), nil, nil},
		{"err: remove missing", "remove", path("dead.beef[0]"), "", notFoundError(path("dead.beef"), doesNotExistMsg), nil, nil},
		{"err: remove obj (non string)", "remove", path("b[100]"), "", notFoundError(path("b[100]"), objectKeyTypeMsg(float64(100))), nil, nil},
		{"err: remove obj (missing)", "remove", path("b.deadbeef"), "", notFoundError(path("b.deadbeef"), doesNotExistMsg), nil, nil},
		{"err: replace root (missing)", "replace", path("deadbeef"), "1", notFoundError(path("deadbeef"), doesNotExistMsg), nil, nil},
		{"err: replace missing", "replace", "dead.beef[1]", "1", notFoundError(path("dead.beef"), doesNotExistMsg), nil, nil},
	}

	for i, tc := range tests {
		data := loadSmallTestData()
		store := NewStorageFromJSONObject(data)

		// Perform patch and check result
		value := loadExpectedSortedResult(tc.value)

		var op StorageOp
		switch tc.op {
		case "add":
			op = StorageAdd
		case "remove":
			op = StorageRemove
		case "replace":
			op = StorageReplace
		default:
			panic(fmt.Sprintf("illegal value: %v", tc.op))
		}

		err := store.Patch(op, path(tc.path), value)

		if tc.expected == nil {
			if err != nil {
				t.Errorf("Test case %d (%v): unexpected patch error: %v", i+1, tc.note, err)
				continue
			}
		} else {
			if err == nil {
				t.Errorf("Test case %d (%v): expected patch error, but got nil instead", i+1, tc.note)
				continue
			}
			if !reflect.DeepEqual(err, tc.expected) {
				t.Errorf("Test case %d (%v): expected patch error %v but got: %v", i+1, tc.note, tc.expected, err)
				continue
			}
		}

		if tc.getPath == nil {
			continue
		}

		// Perform get and verify result
		result, err := store.Get(path(tc.getPath))
		switch expected := tc.getExpected.(type) {
		case error:
			if err == nil {
				t.Errorf("Test case %d (%v): expected get error but got: %v", i+1, tc.note, result)
				continue
			}
			if !reflect.DeepEqual(err, expected) {
				t.Errorf("Test case %d (%v): expected get error %v but got: %v", i+1, tc.note, expected, err)
				continue
			}
		case string:
			if err != nil {
				t.Errorf("Test case %d (%v): unexpected get error: %v", i+1, tc.note, err)
				continue
			}

			e := loadExpectedResult(expected)

			if !reflect.DeepEqual(result, e) {
				t.Errorf("Test case %d (%v): expected get result %v but got: %v", i+1, tc.note, e, result)
			}
		}

	}

}

func path(input interface{}) []interface{} {
	switch input := input.(type) {
	case []interface{}:
		return input
	case string:
		switch v := parseTerm(input).Value.(type) {
		case opalog.Var:
			return []interface{}{string(v)}
		case opalog.Ref:
			path, err := v.Underlying()
			if err != nil {
				panic(err)
			}
			return path
		}
	}
	panic(fmt.Sprintf("illegal value: %v", input))
}
