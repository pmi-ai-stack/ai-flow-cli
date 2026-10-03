package schema

import (
	"bytes"
	"fmt"
	"sync"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v5"

	rawschema "github.com/pmi-ai-stack/ai-flow-cli/schema"
)

// compiled holds one lazily-compiled schema per artifact. Compiling all
// of them up front cost every binary start ~60 ms — paid even by `hook
// guard`, which validates nothing, on every agent tool call — so each
// artifact compiles on its first Validate instead, once per process
// (sync.OnceValue). Per artifact rather than one map-wide Once: a
// command that validates one artifact pays for one compile, not all
// nineteen. Every embedded schema's $refs are in-document ("#/..."),
// so compiling each with its own Compiler holding only its own file is
// equivalent to the former shared one.
//
// A compile failure is a broken embedded schema file — a programmer
// error, never a possible runtime state — so it still panics rather
// than threading an error through every caller (04-errors.md rule 9).
// TestEveryEmbeddedSchemaCompiles forces every entry, so a broken
// schema is still caught by every test run, not by the first user who
// happens to validate that artifact.
var compiled = func() map[string]func() *jsonschema.Schema {
	out := make(map[string]func() *jsonschema.Schema, len(artifacts))
	for _, name := range artifacts {
		out[name] = sync.OnceValue(func() *jsonschema.Schema { return mustCompile(name) })
	}
	return out
}()

func mustCompile(name string) *jsonschema.Schema {
	fname := filename(name)
	data, err := rawschema.FS.ReadFile(fname)
	if err != nil {
		panic(fmt.Sprintf("schema: embedded file %s missing: %v", fname, err))
	}

	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft2020
	if err := compiler.AddResource(fname, bytes.NewReader(data)); err != nil {
		panic(fmt.Sprintf("schema: %s: add resource: %v", fname, err))
	}
	s, err := compiler.Compile(fname)
	if err != nil {
		panic(fmt.Sprintf("schema: %s: compile: %v", fname, err))
	}
	return s
}

// rawSchemaBytes returns the embedded schema document's own bytes for
// artifact, which is assumed already known to the caller.
func rawSchemaBytes(artifact string) ([]byte, error) {
	return rawschema.FS.ReadFile(filename(artifact))
}
