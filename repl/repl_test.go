// Copyright 2016 The OPA Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package repl

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/storage"
)

func TestDump(t *testing.T) {
	input := `{"a": [1,2,3,4]}`
	var data map[string]interface{}
	err := json.Unmarshal([]byte(input), &data)
	if err != nil {
		panic(err)
	}
	store := storage.NewDataStoreFromJSONObject(data)
	var buffer bytes.Buffer
	repl := newRepl(store, &buffer)
	repl.cmdDump()
	expectOutput(t, buffer.String(), "map[a:[1 2 3 4]]\n")
}

func TestOneShotEmptyBufferOneExpr(t *testing.T) {
	store := newTestStorage()
	var buffer bytes.Buffer
	repl := newRepl(store, &buffer)
	repl.OneShot("data.a[i].b.c[j] = 2")
	expectOutput(t, buffer.String(), "+---+---+\n| I | J |\n+---+---+\n| 0 | 1 |\n+---+---+\n")
	buffer.Reset()
	repl.OneShot("data.a[i].b.c[j] = \"deadbeef\"")
	expectOutput(t, buffer.String(), "false\n")
}

func TestOneShotEmptyBufferOneRule(t *testing.T) {
	store := newTestStorage()
	var buffer bytes.Buffer
	repl := newRepl(store, &buffer)
	repl.OneShot("p[x] :- data.a[i] = x")
	expectOutput(t, buffer.String(), "defined\n")
}

func TestOneShotBufferedExpr(t *testing.T) {
	store := newTestStorage()
	var buffer bytes.Buffer
	repl := newRepl(store, &buffer)
	repl.OneShot("data.a[i].b.c[j] = ")
	expectOutput(t, buffer.String(), "")
	repl.OneShot("2")
	expectOutput(t, buffer.String(), "")
	repl.OneShot("")
	expectOutput(t, buffer.String(), "+---+---+\n| I | J |\n+---+---+\n| 0 | 1 |\n+---+---+\n")
}

func TestOneShotBufferedRule(t *testing.T) {
	store := newTestStorage()
	var buffer bytes.Buffer
	repl := newRepl(store, &buffer)
	repl.OneShot("p[x] :- ")
	expectOutput(t, buffer.String(), "")
	repl.OneShot("data.a[i]")
	expectOutput(t, buffer.String(), "")
	repl.OneShot(" = ")
	expectOutput(t, buffer.String(), "")
	repl.OneShot("x")
	expectOutput(t, buffer.String(), "")
	repl.OneShot("")
	expectOutput(t, buffer.String(), "defined\n")
}

func TestOneShotJSON(t *testing.T) {
	store := newTestStorage()
	var buffer bytes.Buffer
	repl := newRepl(store, &buffer)
	repl.OutputFormat = "json"
	repl.OneShot("data.a[i] = x")
	var expected interface{}
	input := `
	[
		{
			"i": 0,
			"x": {
			"b": {
				"c": [
				true,
				2,
				false
				]
			}
			}
		},
		{
			"i": 1,
			"x": {
			"b": {
				"c": [
				false,
				true,
				1
				]
			}
			}
		}
	]
	`
	if err := json.Unmarshal([]byte(input), &expected); err != nil {
		panic(err)
	}

	var result interface{}

	if err := json.Unmarshal(buffer.Bytes(), &result); err != nil {
		t.Errorf("Unexpected output format: %v", err)
		return
	}
	if !reflect.DeepEqual(expected, result) {
		t.Errorf("Expected %v but got: %v", expected, result)
	}
}

func TestBuildHeader(t *testing.T) {
	expr := ast.MustParseStatement(`[{"a": x, "b": data.a.b[y]}] = [{"a": 1, "b": 2}]`).(ast.Body)[0]
	terms := expr.Terms.([]*ast.Term)
	result := map[string]struct{}{}
	buildHeader(result, terms[1])
	expected := map[string]struct{}{
		"x": struct{}{}, "y": struct{}{},
	}
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Build header expected %v but got %v", expected, result)
	}
}

func expectOutput(t *testing.T, output string, expected string) {
	if output != expected {
		t.Errorf("Repl output: expected %#v but got %#v", expected, output)
	}
}

func newRepl(store *storage.DataStore, buffer *bytes.Buffer) *REPL {
	repl := New(store, "", buffer, "")
	return repl
}

func newTestStorage() *storage.DataStore {
	input := `
    {
        "a": [
            {
                "b": {
                    "c": [true,2,false]
                }
            },
            {
                "b": {
                    "c": [false,true,1]
                }
            }
        ]
    }
    `
	var data map[string]interface{}
	err := json.Unmarshal([]byte(input), &data)
	if err != nil {
		panic(err)
	}
	return storage.NewDataStoreFromJSONObject(data)
}
