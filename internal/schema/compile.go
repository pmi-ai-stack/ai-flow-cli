package schema

import (
	"bytes"
	"fmt"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v5"

	rawschema "github.com/pmi-ai-stack/ai-flow-cli/schema"
)

// compiled holds every embedded schema, compiled once at package load.
// A compile failure here is a broken embedded schema file — a
// programmer error caught by every test run, never a possible runtime
// state — so mustCompileAll panics rather than threading an error
// through every caller (04-errors.md rule 9).
var compiled = mustCompileAll()

func mustCompileAll() map[string]*jsonschema.Schema {
	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft2020

	for _, name := range artifacts {
		fname := filename(name)
		data, err := rawschema.FS.ReadFile(fname)
		if err != nil {
			panic(fmt.Sprintf("schema: embedded file %s missing: %v", fname, err))
		}
		if err := compiler.AddResource(fname, bytes.NewReader(data)); err != nil {
			panic(fmt.Sprintf("schema: %s: add resource: %v", fname, err))
		}
	}

	out := make(map[string]*jsonschema.Schema, len(artifacts))
	for _, name := range artifacts {
		fname := filename(name)
		s, err := compiler.Compile(fname)
		if err != nil {
			panic(fmt.Sprintf("schema: %s: compile: %v", fname, err))
		}
		out[name] = s
	}
	return out
}

// rawSchemaBytes returns the embedded schema document's own bytes for
// artifact, which is assumed already known to the caller.
func rawSchemaBytes(artifact string) ([]byte, error) {
	return rawschema.FS.ReadFile(filename(artifact))
}
