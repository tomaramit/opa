// Copyright 2016 The OPA Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package runtime

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/golang/glog"
	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/repl"
	"github.com/open-policy-agent/opa/storage"
	"github.com/pkg/errors"
)

// Params stores the configuration for an OPA instance.
type Params struct {

	// Addr is the listening address that the OPA server will bind to.
	Addr string

	// HistoryPath is the filename to store the interactive shell user
	// input history.
	HistoryPath string

	// Output format controls how the REPL will print query results.
	// Default: "pretty".
	OutputFormat string

	// Paths contains filenames of base documents and policy modules to
	// load on startup.
	Paths []string

	// PolicyDir is the filename of the directory to persist policy
	// definitions in. Policy definitions stored in this directory
	// are automatically loaded on startup.
	PolicyDir string

	// Server flag controls whether the OPA instance will start a server.
	// By default, the OPA instance acts as an interactive shell.
	Server bool
}

// Runtime represents a single OPA instance.
type Runtime struct {
	DataStore   *storage.DataStore
	PolicyStore *storage.PolicyStore
}

// Init initializes the OPA instance.
func (rt *Runtime) Init(params *Params) error {

	if len(params.PolicyDir) > 0 {
		if err := os.MkdirAll(params.PolicyDir, 0755); err != nil {
			return errors.Wrap(err, "unable to make --policy-dir")
		}
	}

	parsed, err := parseInputs(params.Paths)
	if err != nil {
		return errors.Wrapf(err, "parse error")
	}

	// Open data store and load base documents.
	ds := storage.NewDataStore()

	for _, doc := range parsed.docs {
		for k, v := range doc {
			if err := ds.Patch(storage.AddOp, []interface{}{k}, v); err != nil {
				return errors.Wrap(err, "unable to open data store")
			}
		}
	}

	// Open policy store and load existing policies.
	ps := storage.NewPolicyStore(ds, params.PolicyDir)
	if err := ps.Open(storage.LoadPolicies); err != nil {
		return errors.Wrap(err, "unable to open policy store")
	}

	// Load policies provided via input.
	if err := compileAndStoreInputs(parsed.modules, ps); err != nil {
		return errors.Wrapf(err, "compile error")
	}

	rt.PolicyStore = ps
	rt.DataStore = ds

	return nil
}

// Start is the entry point of an OPA instance.
func (rt *Runtime) Start(params *Params) {

	if err := rt.Init(params); err != nil {
		fmt.Println("error initializing runtime:", err)
		os.Exit(1)
	}

	if !params.Server {
		rt.startRepl(params)
	} else {
		rt.startServer(params)
	}
}

func (rt *Runtime) startServer(params *Params) {
	glog.Warning("First line of log stream.")
	glog.V(2).Infof("Server listening address: %v.", params.Addr)
	persist := len(params.PolicyDir) > 0
	server := NewServer(rt, params.Addr, persist)
	server.Loop()
}

func (rt *Runtime) startRepl(params *Params) {
	repl := repl.New(rt.DataStore, params.HistoryPath, os.Stdout, params.OutputFormat)
	repl.Loop()
}

func compileAndStoreInputs(parsed map[string]*parsedModule, policyStore *storage.PolicyStore) error {

	mods := policyStore.List()
	for _, p := range parsed {
		mods[p.id] = p.mod
	}

	c := ast.NewCompiler()
	if c.Compile(mods); c.Failed() {
		// TODO(tsandall): add another call on compiler to flatten into error type
		return c.Errors[0]
	}

	for id := range parsed {
		mod := c.Modules[id]
		if err := policyStore.Add(id, mod, parsed[id].raw, false); err != nil {
			return err
		}
	}

	return nil
}

type parsedModule struct {
	id  string
	mod *ast.Module
	raw []byte
}

type parsedInput struct {
	docs    []map[string]interface{}
	modules map[string]*parsedModule
}

func parseInputs(paths []string) (*parsedInput, error) {

	parsedDocs := []map[string]interface{}{}
	parsedModules := map[string]*parsedModule{}

	for _, file := range paths {

		info, err := os.Stat(file)
		if err != nil {
			return nil, err
		}

		if info.IsDir() {
			continue
		}

		bs, err := ioutil.ReadFile(file)

		if err != nil {
			return nil, err
		}

		m, astErr := ast.ParseModuleFile(file)

		if astErr == nil {
			parsedModules[file] = &parsedModule{
				id:  file,
				mod: m,
				raw: bs,
			}
			continue
		}

		d, jsonErr := parseJSONObjectFile(file)

		if jsonErr == nil {
			parsedDocs = append(parsedDocs, d)
			continue
		}

		switch filepath.Ext(file) {
		case ".json":
			return nil, jsonErr
		case ".rego":
			return nil, astErr
		default:
			return nil, fmt.Errorf("unrecognizable file: %v", file)
		}
	}

	r := &parsedInput{
		docs:    parsedDocs,
		modules: parsedModules,
	}

	return r, nil
}

func parseJSONObjectFile(file string) (map[string]interface{}, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	reader := json.NewDecoder(f)
	var data map[string]interface{}
	if err := reader.Decode(&data); err != nil {
		return nil, err
	}
	return data, nil
}
