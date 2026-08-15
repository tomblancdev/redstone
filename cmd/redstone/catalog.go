package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Kernel holds the two directories everything is read from. All state is
// re-read per request (live-edit pattern) — the files are tiny.
type Kernel struct {
	Dir  string // stacks/, conformance.yaml, graph.jsonl
	Caps string // <capability>.yaml specs

	// Strict: stacked binds (stack != "") must have a declared use.
	Strict bool

	// Serializes graph writes and compaction: an append racing a compaction
	// rename could land a line on the replaced inode and silently vanish.
	mu sync.Mutex
}

// ---------------------------------------------------------------- stacks

// The locked stack-spec: a stack file declares gives (exports) and uses
// (imports). See docs/stack-spec.yaml — this file implements it verbatim.
type StackFile struct {
	Version     int                       `yaml:"version"`
	Description string                    `yaml:"description"`
	Gives       map[string]Give           `yaml:"gives"`
	Uses        map[string]map[string]Use `yaml:"uses"` // app -> task -> use
}

type Give struct {
	Is     string            `yaml:"is"` // capability@level — the claim
	At     string            `yaml:"at"` // in-network URL
	Public string            `yaml:"public"`
	Check  string            `yaml:"check"` // "" or "manifest" | <probe path> | "none"
	Labels map[string]string `yaml:"labels"`
}

// Use is a use-value: "cap@level" | "<give-name>" | full form. Wire values
// are use-values themselves — one grammar, recursively.
type Use struct {
	Use      string         `yaml:"use"`
	Optional bool           `yaml:"optional"`
	With     map[string]any `yaml:"with"`
	Wire     map[string]Use `yaml:"wire"`
}

func (u *Use) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		u.Use = node.Value
		return nil
	}
	// Strict keys: a typo'd field in a use-value (`wih:`) must be an error,
	// never silently-ignored config — the custom unmarshaler bypasses
	// KnownFields, so the check lives here and protects runtime and
	// validate alike.
	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			switch node.Content[i].Value {
			case "use", "optional", "with", "wire":
			default:
				return fmt.Errorf("line %d: unknown field %q in use-value (allowed: use, optional, with, wire)",
					node.Content[i].Line, node.Content[i].Value)
			}
		}
	}
	type plain Use
	return node.Decode((*plain)(u))
}

// IsCapabilityForm reports whether the ref names a capability@level rather
// than a give.
func capabilityForm(ref string) (capability, level string, ok bool) {
	if i := strings.IndexByte(ref, '@'); i > 0 {
		return ref[:i], ref[i+1:], true
	}
	return "", "", false
}

var safeName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// loadStacks reads every stack, sorted by name for deterministic order.
// Both layouts are accepted: stacks/<name>.yaml and stacks/<name>/stack.yaml
// (the spec's folder form, for when stacks carry their compose along).
func (k *Kernel) loadStacks() []struct {
	Name string
	File StackFile
} {
	dir := filepath.Join(k.Dir, "stacks")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	paths := map[string]string{}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			candidate := filepath.Join(dir, e.Name(), "stack.yaml")
			if _, err := os.Stat(candidate); err == nil {
				paths[e.Name()] = candidate
				names = append(names, e.Name())
			}
		} else if strings.HasSuffix(e.Name(), ".yaml") {
			name := strings.TrimSuffix(e.Name(), ".yaml")
			paths[name] = filepath.Join(dir, e.Name())
			names = append(names, name)
		}
	}
	sort.Strings(names)

	var out []struct {
		Name string
		File StackFile
	}
	for _, name := range names {
		data, err := os.ReadFile(paths[name])
		if err != nil {
			continue
		}
		var doc StackFile
		if err := yaml.Unmarshal(data, &doc); err != nil {
			fmt.Printf("stack %s: unparsable, skipped (%v)\n", name, err)
			continue
		}
		out = append(out, struct {
			Name string
			File StackFile
		}{name, doc})
	}
	return out
}

// usesFor returns one stack's declared uses (nil when absent). The stack
// name arrives from requests and becomes a filename: sanitize.
func (k *Kernel) usesFor(stack string) map[string]map[string]Use {
	if stack == "" || !safeName.MatchString(stack) {
		return nil
	}
	for _, s := range k.loadStacks() {
		if s.Name == stack {
			return s.File.Uses
		}
	}
	return nil
}

// ---------------------------------------------------------------- catalog

type Instance struct {
	Name       string            `json:"name"`
	Capability string            `json:"capability"`
	Level      string            `json:"level"`
	Endpoint   string            `json:"endpoint"`
	Public     string            `json:"public"`
	Labels     map[string]string `json:"labels"`
	Check      string            `json:"check,omitempty"`
	Stack      string            `json:"stack"`

	// Merged from conformance verdicts, never from the file:
	Verified       bool    `json:"verified"`
	EffectiveLevel *string `json:"effective_level"`
}

func (k *Kernel) loadInstances() []Instance {
	var all []Instance
	seen := map[string]bool{}
	for _, s := range k.loadStacks() {
		var giveNames []string
		for name := range s.File.Gives {
			giveNames = append(giveNames, name)
		}
		sort.Strings(giveNames)
		for _, name := range giveNames {
			if seen[name] {
				fmt.Printf("give %s: name clash, first stack wins\n", name)
				continue
			}
			give := s.File.Gives[name]
			capability, level, ok := capabilityForm(give.Is)
			if !ok {
				continue
			}
			seen[name] = true
			all = append(all, Instance{
				Name: name, Capability: capability, Level: level,
				Endpoint: give.At, Public: give.Public,
				Labels: give.Labels, Check: give.Check, Stack: s.Name,
			})
		}
	}
	return all
}

type TestResult struct {
	Name  string `yaml:"name" json:"name"`
	Ok    bool   `yaml:"ok" json:"ok"`
	Error string `yaml:"error,omitempty" json:"error,omitempty"`
}

type Verdict struct {
	Capability    string       `yaml:"capability" json:"capability"`
	Claimed       string       `yaml:"claimed" json:"claimed"`
	VerifiedLevel *string      `yaml:"verified_level" json:"verified_level"`
	Tests         []TestResult `yaml:"tests" json:"tests"`
}

type conformanceFile struct {
	GeneratedAt string             `yaml:"generated_at" json:"generated_at"`
	Results     map[string]Verdict `yaml:"results" json:"results"`
}

// VerdictsDocument serves the conformance file as JSON — the `verdicts`
// operation of the conformance core capability.
func (k *Kernel) VerdictsDocument() conformanceFile {
	var doc conformanceFile
	data, err := os.ReadFile(filepath.Join(k.Dir, "conformance.yaml"))
	if err == nil {
		yaml.Unmarshal(data, &doc)
	}
	if doc.Results == nil {
		doc.Results = map[string]Verdict{}
	}
	return doc
}

func (k *Kernel) loadVerdicts() map[string]Verdict {
	return k.VerdictsDocument().Results
}

// Catalog merges claims with verdicts: the verdict wins where one exists.
func (k *Kernel) Catalog() []Instance {
	verdicts := k.loadVerdicts()
	instances := k.loadInstances()
	for i := range instances {
		if v, ok := verdicts[instances[i].Name]; ok {
			instances[i].Verified = true
			instances[i].EffectiveLevel = v.VerifiedLevel
		} else {
			level := instances[i].Level
			instances[i].EffectiveLevel = &level
		}
	}
	return instances
}

// Ladder is the ordered level list of a capability spec — YAML key order,
// weakest first. nil when no spec exists (claimed level = one-rung ladder).
func (k *Kernel) Ladder(capability string) []string {
	data, err := os.ReadFile(filepath.Join(k.Caps, capability+".yaml"))
	if err != nil {
		return nil
	}
	var doc yaml.Node
	if yaml.Unmarshal(data, &doc) != nil || len(doc.Content) == 0 {
		return nil
	}
	root := doc.Content[0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "levels" {
			levels := root.Content[i+1]
			var steps []string
			for j := 0; j+1 < len(levels.Content); j += 2 {
				steps = append(steps, levels.Content[j].Value)
			}
			return steps
		}
	}
	return nil
}

func rank(steps []string, level string) int {
	if steps == nil {
		return 0
	}
	for i, s := range steps {
		if s == level {
			return i
		}
	}
	return -1
}

// crossCheck: the winner must be alive, and — when it speaks our envelope —
// agree on the CAPABILITY. `check` selects the mode per the spec:
// "manifest" (default) full cross-check | <probe path> liveness GET | "none".
// Non-HTTP endpoints are unprobeable by design.
func crossCheck(inst Instance) (flags map[string]any, implementation *string, reason string) {
	client := http.Client{Timeout: 2500 * time.Millisecond}

	if inst.Check == "none" || !strings.HasPrefix(inst.Endpoint, "http") {
		return map[string]any{}, nil, ""
	}
	if inst.Check != "" && inst.Check != "manifest" {
		resp, err := client.Get(inst.Endpoint + inst.Check)
		if err != nil {
			return nil, nil, "unreachable"
		}
		resp.Body.Close()
		if resp.StatusCode >= 500 {
			return nil, nil, fmt.Sprintf("probe %d", resp.StatusCode)
		}
		return map[string]any{}, nil, ""
	}

	resp, err := client.Get(inst.Endpoint + "/manifest")
	if err != nil {
		return nil, nil, "unreachable"
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Sprintf("manifest %d", resp.StatusCode)
	}
	var m struct {
		Capability     string         `json:"capability"`
		Implementation *string        `json:"implementation"`
		Flags          map[string]any `json:"flags"`
	}
	if json.NewDecoder(resp.Body).Decode(&m) != nil {
		return nil, nil, "manifest unparsable"
	}
	if m.Capability != inst.Capability {
		return nil, nil, fmt.Sprintf("manifest says capability=%s, registry says %s", m.Capability, inst.Capability)
	}
	if m.Flags == nil {
		m.Flags = map[string]any{}
	}
	return m.Flags, m.Implementation, ""
}
