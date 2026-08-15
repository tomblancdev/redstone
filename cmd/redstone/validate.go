package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Registry validation — the enforcement layer for YAML typos, the most
// probable daily failure for a file-driven kernel. Used by `redstone validate`,
// by `redstone add` (as its gate), and at serve boot (as warnings).
//
// Severity: an "error" is something that will misbehave at bind time
// (unbindable instance, unresolvable reference, unknown field); a "warn" is
// legal but worth a look (no spec = claimed trust, unusual scheme...).
type Issue struct {
	Severity string // "error" | "warn"
	Where    string
	Msg      string
}

func (k *Kernel) Validate() []Issue {
	var issues []Issue
	add := func(severity, where, format string, args ...any) {
		issues = append(issues, Issue{severity, where, fmt.Sprintf(format, args...)})
	}

	dir := filepath.Join(k.Dir, "stacks")
	entries, err := os.ReadDir(dir)
	if err != nil {
		add("error", "registry", "cannot read stacks dir: %v", err)
		return issues
	}

	type parsedStack struct {
		Name string
		File StackFile
	}
	var stacks []parsedStack
	giveOwner := map[string]string{} // give name -> stack

	var names []string
	paths := map[string]string{}
	for _, e := range entries {
		if e.IsDir() {
			candidate := filepath.Join(dir, e.Name(), "stack.yaml")
			if _, err := os.Stat(candidate); err == nil {
				names = append(names, e.Name())
				paths[e.Name()] = candidate
			}
		} else if strings.HasSuffix(e.Name(), ".yaml") {
			name := strings.TrimSuffix(e.Name(), ".yaml")
			names = append(names, name)
			paths[name] = filepath.Join(dir, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		where := name
		if !safeName.MatchString(name) {
			add("error", where, "stack name must match [A-Za-z0-9][A-Za-z0-9_-]*")
			continue
		}
		data, err := os.ReadFile(paths[name])
		if err != nil {
			add("error", where, "unreadable: %v", err)
			continue
		}
		// Strict decoding: unknown fields (a typo'd `wih:`, a stray `give:`)
		// are ERRORS here even though the runtime loader is lenient.
		var doc StackFile
		dec := yaml.NewDecoder(bytes.NewReader(data))
		dec.KnownFields(true)
		if err := dec.Decode(&doc); err != nil {
			add("error", where, "invalid stack file: %v", err)
			continue
		}
		if doc.Version != 1 {
			add("warn", where, "version is %d, expected 1", doc.Version)
		}

		var giveNames []string
		for g := range doc.Gives {
			giveNames = append(giveNames, g)
		}
		sort.Strings(giveNames)
		for _, giveName := range giveNames {
			give := doc.Gives[giveName]
			gw := where + "/gives/" + giveName
			if !safeName.MatchString(giveName) {
				add("error", gw, "give name must match [A-Za-z0-9][A-Za-z0-9_-]*")
			}
			if owner, clash := giveOwner[giveName]; clash {
				add("error", gw, "name already given by stack %q (first in sorted order wins)", owner)
			} else {
				giveOwner[giveName] = name
			}
			capability, level, ok := capabilityForm(give.Is)
			if !ok {
				add("error", gw, "is: %q is not capability@level", give.Is)
			} else if steps := k.Ladder(capability); steps == nil {
				add("warn", gw, "no spec for capability %q — binds run on claimed trust", capability)
			} else if rank(steps, level) < 0 {
				add("error", gw, "level %q is not on %s's ladder %v — this instance can NEVER bind", level, capability, steps)
			}
			if give.At == "" {
				add("error", gw, "at: is required")
			} else if !strings.HasPrefix(give.At, "http") && !strings.HasPrefix(give.At, "tcp://") {
				add("warn", gw, "at: %q has an unusual scheme", give.At)
			}
			if give.Check != "" && give.Check != "manifest" && give.Check != "none" && !strings.HasPrefix(give.Check, "/") {
				add("warn", gw, "check: %q is not manifest|none|<path>", give.Check)
			}
		}
		stacks = append(stacks, parsedStack{name, doc})
	}

	// Second pass: uses may reference any stack's gives.
	var checkUse func(where string, u Use, depth int)
	checkUse = func(where string, u Use, depth int) {
		if depth > maxWireDepth {
			add("error", where, "wire nesting exceeds max depth %d", maxWireDepth)
			return
		}
		if u.Use == "" && len(u.Wire) == 0 && len(u.With) == 0 {
			add("warn", where, "empty use-value")
			return
		}
		if u.Use != "" {
			if capability, level, ok := capabilityForm(u.Use); ok {
				if steps := k.Ladder(capability); steps == nil {
					add("warn", where, "no spec for capability %q — claimed trust", capability)
				} else if rank(steps, level) < 0 {
					add("error", where, "level %q is not on %s's ladder %v", level, capability, steps)
				}
			} else if _, exists := giveOwner[u.Use]; !exists {
				add("error", where, "references give %q which no stack provides", u.Use)
			}
		}
		var wireTasks []string
		for t := range u.Wire {
			wireTasks = append(wireTasks, t)
		}
		sort.Strings(wireTasks)
		for _, t := range wireTasks {
			if !safeName.MatchString(t) {
				add("error", where+"/wire/"+t, "task name must match [A-Za-z0-9][A-Za-z0-9_-]*")
			}
			checkUse(where+"/wire/"+t, u.Wire[t], depth+1)
		}
	}

	for _, s := range stacks {
		var apps []string
		for a := range s.File.Uses {
			apps = append(apps, a)
		}
		sort.Strings(apps)
		for _, app := range apps {
			if !safeName.MatchString(app) {
				add("error", s.Name+"/uses/"+app, "app name must match [A-Za-z0-9][A-Za-z0-9_-]*")
			}
			var tasks []string
			for t := range s.File.Uses[app] {
				tasks = append(tasks, t)
			}
			sort.Strings(tasks)
			for _, task := range tasks {
				where := s.Name + "/uses/" + app + "." + task
				if !safeName.MatchString(task) {
					add("error", where, "task name must match [A-Za-z0-9][A-Za-z0-9_-]*")
				}
				checkUse(where, s.File.Uses[app][task], 0)
			}
		}
	}
	return issues
}

// PrintIssues renders issues and reports whether any is an error.
func PrintIssues(issues []Issue) bool {
	hasError := false
	for _, issue := range issues {
		fmt.Printf("%-5s %-40s %s\n", issue.Severity, issue.Where, issue.Msg)
		if issue.Severity == "error" {
			hasError = true
		}
	}
	if len(issues) == 0 {
		fmt.Println("⚡ all circuits clean — registry valid")
	}
	return hasError
}
