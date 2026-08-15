package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const maxWireDepth = 3

type BindParams struct {
	Capability string
	Level      string
	Name       string
	Labels     map[string]string
	Consumer   string
	As         string
	Stack      string
}

type BindOK struct {
	Name           string         `json:"name"`
	Capability     string         `json:"capability"`
	RequestedLevel string         `json:"requested_level"`
	EffectiveLevel *string        `json:"effective_level"`
	Verified       bool           `json:"verified"`
	Endpoint       string         `json:"endpoint"`
	Public         string         `json:"public"`
	Implementation *string        `json:"implementation"`
	Flags          map[string]any `json:"flags"`
}

type Candidate struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type Refusal struct {
	Error      string      `json:"error"`
	Candidates []Candidate `json:"candidates"`
}

type Edge struct {
	Stack      string         `json:"stack,omitempty"`
	Consumer   string         `json:"consumer"`
	As         string         `json:"as"`
	Capability string         `json:"capability"`
	Level      string         `json:"level"`
	Instance   string         `json:"instance"`
	Verified   bool           `json:"verified"`
	At         string         `json:"at"`
	Optional   bool           `json:"optional,omitempty"`
	With       map[string]any `json:"with,omitempty"`
	Wire       map[string]any `json:"wire,omitempty"`
}

// pick filters the catalog for a target: by name (pin) or capability+level
// (+labels), self-first. Returns ordered candidates plus recorded rejections.
func (k *Kernel) pick(catalog []Instance, steps []string, stack, name, capability, level string, labels map[string]string) (candidates []Instance, rejected []Candidate) {
	var own, fleet []Instance
	for _, inst := range catalog {
		if capability != "" && inst.Capability != capability {
			continue
		}
		if name != "" && inst.Name != name {
			continue
		}
		labelMismatch := false
		for key, want := range labels {
			got, ok := inst.Labels[key]
			if !ok || got != want {
				display := "unset"
				if ok {
					display = got
				}
				rejected = append(rejected, Candidate{inst.Name, fmt.Sprintf("label %s=%s, wanted %s", key, display, want)})
				labelMismatch = true
				break
			}
		}
		if labelMismatch {
			continue
		}
		if inst.EffectiveLevel == nil || rank(steps, *inst.EffectiveLevel) < rank(steps, level) {
			effective := "nothing"
			if inst.EffectiveLevel != nil {
				effective = *inst.EffectiveLevel
			}
			reason := fmt.Sprintf("provides %s, %s requested", effective, level)
			if inst.Verified {
				reason = fmt.Sprintf("claims %s, conformance verified only %s", inst.Level, effective)
			}
			rejected = append(rejected, Candidate{inst.Name, reason})
			continue
		}
		// Lexical scoping: a stack's own gives resolve first, then the fleet.
		if stack != "" && inst.Stack == stack {
			own = append(own, inst)
		} else {
			fleet = append(fleet, inst)
		}
	}
	return append(own, fleet...), rejected
}

// resolveWire resolves one wire use-value (and its nested wires) to a bound
// result map. Same verification as any bind; depth-limited; cycles refused.
func (k *Kernel) resolveWire(catalog []Instance, stack string, u Use, depth int, visited map[string]bool) (map[string]any, string) {
	if depth > maxWireDepth {
		return nil, fmt.Sprintf("wire depth exceeds %d", maxWireDepth)
	}

	name, capability, level := "", "", ""
	if c, l, ok := capabilityForm(u.Use); ok {
		capability, level = c, l
	} else {
		name = u.Use
		for _, inst := range catalog {
			if inst.Name == name {
				capability, level = inst.Capability, inst.Level
				break
			}
		}
		if capability == "" {
			return nil, fmt.Sprintf("no give named %q", name)
		}
	}
	if visited[u.Use] {
		return nil, fmt.Sprintf("wiring cycle at %q", u.Use)
	}
	visited[u.Use] = true
	defer delete(visited, u.Use)

	steps := k.Ladder(capability)
	candidates, _ := k.pick(catalog, steps, stack, name, capability, level, nil)
	for _, inst := range candidates {
		if _, _, reason := crossCheck(inst); reason != "" {
			continue
		}
		result := map[string]any{
			"name": inst.Name, "capability": inst.Capability,
			"endpoint": inst.Endpoint, "public": inst.Public,
			"verified": inst.Verified,
		}
		if len(u.With) > 0 {
			result["with"] = u.With
		}
		if len(u.Wire) > 0 {
			nested := map[string]any{}
			for task, sub := range u.Wire {
				out, reason := k.resolveWire(catalog, stack, sub, depth+1, visited)
				if reason != "" {
					if sub.Optional {
						continue
					}
					return nil, fmt.Sprintf("%s: %s", task, reason)
				}
				nested[task] = out
			}
			result["wire"] = nested
		}
		return result, ""
	}
	return nil, fmt.Sprintf("no instance satisfies %s@%s", capability, level)
}

// SelectInstance is the binder — the locked stack-spec's semantics:
//  1. the stack's declared use (if any) shapes the request: name form pins,
//     capability form drift-checks, full form adds with/wire
//  2. own gives first, then fleet; verdicts gate every path
//  3. the winner must pass its check (manifest cross-check or probe)
//  4. an edge (with with/wire) is recorded on the graph
func (k *Kernel) SelectInstance(p BindParams) (*BindOK, *Refusal, string) {
	alias := p.As
	if alias == "" {
		alias = p.Capability
	}
	if alias == "" {
		return nil, nil, "capability and level are required (or a task with a declared use)"
	}

	catalog := k.Catalog()

	// The stack's declared use, if one exists for this app+task. It is the
	// single source of truth: a request may omit capability/level entirely
	// and inherit them from the declaration (declaration-driven bind); a
	// request that states them must agree with it (drift refusal).
	var declared *Use
	if uses := k.usesFor(p.Stack); uses != nil {
		if tasks, ok := uses[p.Consumer]; ok {
			if u, ok := tasks[alias]; ok {
				declared = &u
			} else if p.Capability != "" {
				if u, ok := tasks[p.Capability]; ok {
					declared = &u
				}
			}
		}
	}
	if k.Strict && p.Stack != "" && declared == nil {
		return nil, nil, fmt.Sprintf("strict mode: no declared use for %s/%s.%s — declare it in the stack file",
			p.Stack, p.Consumer, alias)
	}
	if declared != nil && declared.Use != "" {
		declCap, declLevel, isCapForm := capabilityForm(declared.Use)
		if !isCapForm {
			// Name form: the declared give defines capability and level.
			for _, inst := range catalog {
				if inst.Name == declared.Use {
					declCap, declLevel = inst.Capability, inst.Level
					break
				}
			}
			if declCap == "" {
				return nil, nil, fmt.Sprintf("stack '%s' wires %s.%s to unknown give %q",
					p.Stack, p.Consumer, alias, declared.Use)
			}
		}
		if p.Capability == "" {
			p.Capability = declCap
		} else if p.Capability != declCap {
			return nil, nil, fmt.Sprintf("stack '%s' declares %s.%s as %s, request asked %s",
				p.Stack, p.Consumer, alias, declCap, p.Capability)
		}
		if p.Level == "" {
			p.Level = declLevel
		} else if declLevel != "" && p.Level != declLevel {
			return nil, nil, fmt.Sprintf("stack '%s' declares %s.%s at level %s, request asked %s",
				p.Stack, p.Consumer, alias, declLevel, p.Level)
		}
		if !isCapForm {
			p.Name, p.Labels = declared.Use, nil
		}
	}

	if p.Capability == "" || p.Level == "" {
		return nil, nil, fmt.Sprintf("capability and level are required (no declared use for %s/%s.%s)",
			p.Stack, p.Consumer, alias)
	}
	steps := k.Ladder(p.Capability)
	if steps != nil && rank(steps, p.Level) < 0 {
		return nil, nil, fmt.Sprintf("unknown level '%s' for %s (levels: %v)", p.Level, p.Capability, steps)
	}

	candidates, rejected := k.pick(catalog, steps, p.Stack, p.Name, p.Capability, p.Level, p.Labels)

	for _, inst := range candidates {
		flags, implementation, reason := crossCheck(inst)
		if reason != "" {
			rejected = append(rejected, Candidate{inst.Name, reason})
			continue
		}

		// Resolve per-edge wiring before committing the edge.
		var wired map[string]any
		if declared != nil && len(declared.Wire) > 0 {
			wired = map[string]any{}
			failed := false
			for task, sub := range declared.Wire {
				out, reason := k.resolveWire(catalog, p.Stack, sub, 1, map[string]bool{})
				if reason != "" {
					if sub.Optional {
						continue
					}
					rejected = append(rejected, Candidate{inst.Name, fmt.Sprintf("wire %s: %s", task, reason)})
					failed = true
					break
				}
				wired[task] = out
			}
			if failed {
				continue
			}
		}

		consumer := p.Consumer
		if consumer == "" {
			consumer = "anonymous"
		}
		edge := Edge{
			Stack: p.Stack, Consumer: consumer, As: alias,
			Capability: p.Capability, Level: p.Level,
			Instance: inst.Name, Verified: inst.Verified,
			At: time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		}
		if declared != nil {
			edge.With = declared.With
		}
		edge.Wire = wired
		k.appendEdge(edge)

		return &BindOK{
			Name: inst.Name, Capability: p.Capability, RequestedLevel: p.Level,
			EffectiveLevel: inst.EffectiveLevel, Verified: inst.Verified,
			Endpoint: inst.Endpoint, Public: inst.Public,
			Implementation: implementation, Flags: flags,
		}, nil, ""
	}

	return nil, &Refusal{
		Error:      fmt.Sprintf("no instance satisfies %s@%s", p.Capability, p.Level),
		Candidates: append([]Candidate{}, rejected...),
	}, ""
}

// compactThreshold: past this size the hot graph file is folded on the next
// append. A crash-looping consumer rebinding forever is the realistic way to
// grow it; compaction keeps /graph reads O(current wiring), not O(history).
const compactThreshold = 1 << 20 // 1 MiB

func (k *Kernel) appendEdge(e Edge) {
	k.mu.Lock()
	defer k.mu.Unlock()
	path := filepath.Join(k.Dir, "graph.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	line, _ := json.Marshal(e)
	f.Write(append(line, '\n'))
	f.Close()

	if info, err := os.Stat(path); err == nil && info.Size() > compactThreshold {
		kept, archived := k.compactLocked()
		fmt.Printf("graph compacted: %d kept, %d archived\n", kept, archived)
	}
}

// CompactGraph folds the hot file latest-wins per (stack|consumer|as) and
// moves superseded lines to graph-archive.jsonl — history is preserved as a
// cold audit trail, the hot file stays the size of the CURRENT wiring.
// Runs at serve start and past the size threshold on append.
func (k *Kernel) CompactGraph() (kept, archived int) {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.compactLocked()
}

func (k *Kernel) compactLocked() (kept, archived int) {
	path := filepath.Join(k.Dir, "graph.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, 0
	}
	lines := splitLines(data)

	latest := map[string]int{} // key -> index of last line for that key
	for i, line := range lines {
		var e Edge
		if json.Unmarshal(line, &e) != nil {
			continue
		}
		latest[e.Stack+"|"+e.Consumer+"|"+e.As] = i
	}
	keep := map[int]bool{}
	for _, i := range latest {
		keep[i] = true
	}
	if len(keep) == len(lines) {
		return len(lines), 0
	}

	var hot, cold []byte
	for i, line := range lines {
		if keep[i] {
			hot = append(hot, append(line, '\n')...)
			kept++
		} else {
			cold = append(cold, append(line, '\n')...)
			archived++
		}
	}
	if archive, err := os.OpenFile(filepath.Join(k.Dir, "graph-archive.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
		archive.Write(cold)
		archive.Close()
	}
	tmp := path + ".tmp"
	if os.WriteFile(tmp, hot, 0o644) == nil {
		os.Rename(tmp, path)
	}
	return kept, archived
}

// EdgeFor returns the current bound edge for (stack, app, task) — the
// `edge` operation adapters pull their with/wire config from.
func (k *Kernel) EdgeFor(stack, app, task string) *Edge {
	for _, e := range k.boundEdges() {
		if e.Stack == stack && e.Consumer == app && e.As == task {
			return &e
		}
	}
	return nil
}

func (k *Kernel) boundEdges() []Edge {
	data, err := os.ReadFile(filepath.Join(k.Dir, "graph.jsonl"))
	if err != nil {
		return nil
	}
	latest := map[string]Edge{}
	var order []string
	for _, line := range splitLines(data) {
		var e Edge
		if json.Unmarshal(line, &e) != nil {
			continue
		}
		key := e.Stack + "|" + e.Consumer + "|" + e.As
		if _, seen := latest[key]; !seen {
			order = append(order, key)
		}
		latest[key] = e
	}
	edges := make([]Edge, 0, len(order))
	for _, key := range order {
		edges = append(edges, latest[key])
	}
	return edges
}

// GraphEdges: declared edges first (every stack's uses with their LIVE
// resolution, at: "declared", wire sub-edges as task paths), then the bound
// edges, latest-wins per (stack, consumer, as).
func (k *Kernel) GraphEdges() []Edge {
	var edges []Edge
	catalog := k.Catalog()

	for _, s := range k.loadStacks() {
		for app, tasks := range s.File.Uses {
			for task, u := range tasks {
				edges = append(edges, k.declaredEdges(catalog, s.Name, app, task, u, 0)...)
			}
		}
	}
	return append(edges, k.boundEdges()...)
}

func (k *Kernel) declaredEdges(catalog []Instance, stack, app, taskPath string, u Use, depth int) []Edge {
	if depth > maxWireDepth {
		return nil
	}
	capability, level := "", ""
	if c, l, ok := capabilityForm(u.Use); ok {
		capability, level = c, l
	} else {
		for _, inst := range catalog {
			if inst.Name == u.Use {
				capability, level = inst.Capability, inst.Level
				break
			}
		}
	}
	resolved, verified := "unresolved", false
	if capability != "" {
		steps := k.Ladder(capability)
		name := ""
		if _, _, ok := capabilityForm(u.Use); !ok {
			name = u.Use
		}
		if candidates, _ := k.pick(catalog, steps, stack, name, capability, level, nil); len(candidates) > 0 {
			resolved, verified = candidates[0].Name, candidates[0].Verified
		}
	}
	edges := []Edge{{
		Stack: stack, Consumer: app, As: taskPath,
		Capability: capability, Level: level,
		Instance: resolved, Verified: verified,
		At: "declared", Optional: u.Optional,
	}}
	for wireTask, sub := range u.Wire {
		edges = append(edges, k.declaredEdges(catalog, stack, app, taskPath+"/"+wireTask, sub, depth+1)...)
	}
	return edges
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			if i > start {
				lines = append(lines, data[start:i])
			}
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
