// Package tool defines the Tool abstraction and a Registry. Built-in tools live
// in tool/builtin and self-register via init(); plugin-provided tools are added
// to a runtime Registry alongside the enabled built-ins. The agent sees only a
// *Registry, never the global built-in set directly.
package tool

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"tianxuan/internal/diff"
	"tianxuan/internal/provider"
)

// Tool is a capability the model can invoke.
type Tool interface {
	Name() string
	Description() string
	// Schema returns the JSON Schema for the tool's parameters.
	Schema() json.RawMessage
	// Execute parses the model-generated raw JSON args and returns result text
	// to feed back to the model.
	Execute(ctx context.Context, args json.RawMessage) (string, error)
	// ReadOnly reports whether the tool has no observable side effects on the
	// host. The agent parallelises a batch of tool calls only when every call
	// in the batch is ReadOnly; mixed batches stay sequential so write/read
	// ordering is preserved. bash and plugin tools must return false because
	// their effects can't be inferred statically from args.
	ReadOnly() bool
}

// CompactDescriptor is an optional capability a Tool may implement. When present,
// CompactDescription replaces Description and CompactSchema replaces Schema in
// the provider-facing tool list, significantly reducing per-turn prompt tokens.
// Tools that don't implement this fall back to their full Description + Schema.
type CompactDescriptor interface {
	CompactDescription() string
	CompactSchema() json.RawMessage
}

// Previewer is an optional capability a writer Tool may implement: given the
// same raw JSON args Execute would receive, compute the file change the call
// *would* make — without touching disk. A front-end uses it to show an approval
// card or a changed-files panel before the call runs (the permission gate, not
// Preview, decides whether it may proceed). Type-assert a Tool to Previewer to
// discover support; the file-writing built-ins implement it, most tools do not.
type Previewer interface {
	Preview(args json.RawMessage) (diff.Change, error)
}

// --- process-global built-in set (populated by builtin subpackage init) ---

var builtins = map[string]Tool{}

// RegisterBuiltin registers a compile-time built-in tool. Intended for init().
// It panics on a duplicate name, which is a compile-time wiring mistake.
func RegisterBuiltin(t Tool) {
	name := t.Name()
	if _, dup := builtins[name]; dup {
		panic("tool: duplicate built-in " + name)
	}
	builtins[name] = t
}

// Builtins returns all registered built-in tools, sorted by name.
func Builtins() []Tool {
	names := make([]string, 0, len(builtins))
	for n := range builtins {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]Tool, 0, len(names))
	for _, n := range names {
		out = append(out, builtins[n])
	}
	return out
}

// LookupBuiltin returns a registered built-in by name.
func LookupBuiltin(name string) (Tool, bool) {
	t, ok := builtins[name]
	return t, ok
}

// --- per-run registry instance ---

// Registry is a per-run set of tools: enabled built-ins plus plugin tools.
// V6.0 P8: supports hiding tools from the model schema while keeping them callable.
type Registry struct {
	tools  map[string]Tool
	order  []string
	hidden map[string]bool            // V6.0 P8: hidden from schema but still callable
	canon     map[string]json.RawMessage // V10.0: schema canonicalized once on Add, reused by Schemas()
	suspended map[string]bool            // V10.0: MCP prefixes temporarily disabled per-session
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{tools: map[string]Tool{}, hidden: map[string]bool{}, canon: map[string]json.RawMessage{}, suspended: map[string]bool{}}
}

// Add inserts (or replaces) a tool, preserving first-seen order.
// V10.0: canonicalizes the schema once here — Schemas() reuses the cached result.
func (r *Registry) Add(t Tool) {
	name := t.Name()
	if _, ok := r.tools[name]; !ok {
		r.order = append(r.order, name)
	}
	for prefix := range r.suspended {
		if strings.HasPrefix(name, prefix) {
			return // silently reject — prefix is suspended
		}
	}
	r.tools[name] = t
	r.canon[name] = provider.CanonicalizeSchema(t.Schema())
}

// Hide removes a tool from the model-visible schema list without unregistering it.
// Hidden tools remain callable via Get(). V6.0 P8: reduces model cognitive load.
func (r *Registry) Hide(name string) {
	r.hidden[name] = true
}

// HideUnlessOnly hides each given name only when the registry also contains at
// least one of the alternatives — so the model always has at least one way to
// perform the operation. V6.0 P8.
func (r *Registry) HideUnlessOnly(names []string, alternatives []string) {
	hasAlt := false
	for _, a := range alternatives {
		if _, ok := r.tools[a]; ok {
			hasAlt = true
			break
		}
	}
	if !hasAlt {
		return // don't hide if no alternative available
	}
	for _, n := range names {
		r.hidden[n] = true
	}
}

// MCPNamePrefix is the namespace every MCP tool name carries: the
// model-visible name is "mcp__<server>__<tool>".
const MCPNamePrefix = "mcp__"

// SplitMCPName splits a model-visible MCP tool name "mcp__<server>__<tool>" into
// its server and tool parts. ok is false for non-MCP (built-in) names and for
// malformed names missing either part.
func SplitMCPName(name string) (server, tool string, ok bool) {
	if !strings.HasPrefix(name, MCPNamePrefix) {
		return "", "", false
	}
	rest := name[len(MCPNamePrefix):]
	parts := strings.SplitN(rest, "__", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// RemovePrefix unregisters every tool whose name starts with prefix — used to
// drop an MCP server's "mcp__<server>__" namespace when it's disconnected — and
// returns the count removed.
func (r *Registry) RemovePrefix(prefix string) int {
	kept := r.order[:0]
	removed := 0
	for _, name := range r.order {
		if strings.HasPrefix(name, prefix) {
			delete(r.tools, name)
			delete(r.canon, name)
			removed++
			continue
		}
		kept = append(kept, name)
	}
	r.order = kept
	return removed
}

// SuspendPrefix unregisters every tool whose name starts with prefix, and
// prevents future Add calls for that prefix until ResumePrefix is called.
// Used for per-session MCP disables — an in-flight background handshake
// may attempt to re-add tools for the suspended prefix.
func (r *Registry) SuspendPrefix(prefix string) int {
	r.suspended[prefix] = true
	kept := r.order[:0]
	removed := 0
	for _, name := range r.order {
		if strings.HasPrefix(name, prefix) {
			delete(r.tools, name)
			delete(r.canon, name)
			removed++
			continue
		}
		kept = append(kept, name)
	}
	r.order = kept
	return removed
}

// ResumePrefix allows future Add calls for a previously suspended prefix.
func (r *Registry) ResumePrefix(prefix string) {
	delete(r.suspended, prefix)
}

// Get looks up a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// Len returns the number of registered tools.
func (r *Registry) Len() int { return len(r.order) }

// Names returns the registered tool names in insertion order.
func (r *Registry) Names() []string {
	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}

// Schemas exports tool definitions in stable name order for the provider.
// When a tool implements CompactDescriptor, the compact versions are used
// instead of the full Description + Schema, reducing per-turn prompt tokens.
// V6.0 P8: hidden tools are excluded from the schema list.
// V10.0: standard schemas use pre-canonicalized cache from Add().
func (r *Registry) Schemas() []provider.ToolSchema {
	return r.FilteredSchemas(nil)
}

// FilteredSchemas is like Schemas but only includes tools whose names appear
// in the names slice. When names is nil or empty, all non-hidden tools are
// included (equivalent to Schemas()). Tools not found in the registry are
// silently skipped.
func (r *Registry) FilteredSchemas(names []string) []provider.ToolSchema {
	allNames := r.Names()
	sort.Strings(allNames)

	var filter map[string]bool
	if len(names) > 0 {
		filter = make(map[string]bool, len(names))
		for _, n := range names {
			filter[n] = true
		}
	}

	out := make([]provider.ToolSchema, 0, len(allNames))
	for _, name := range allNames {
		if r.hidden[name] {
			continue
		}
		if filter != nil && !filter[name] {
			continue
		}
		t := r.tools[name]
		desc := t.Description()
		if cd, ok := t.(CompactDescriptor); ok {
			desc = cd.CompactDescription()
			schema := cd.CompactSchema()
			// Compact schemas are context-dependent — canonicalize inline.
			out = append(out, provider.ToolSchema{
				Name:        t.Name(),
				Description: desc,
				Parameters:  provider.CanonicalizeSchema(schema),
			})
		} else {
			// Standard schema — use pre-canonicalized cache from Add().
			out = append(out, provider.ToolSchema{
				Name:        t.Name(),
				Description: desc,
				Parameters:  r.canon[name],
			})
		}
	}
	return out
}
