// Redstone — a capability kernel, as one portable static binary.
//
// Server:
//
//	redstone serve    the register: catalog, bind, graph. HTTP+JSON and gRPC.
//	                --strict refuses stacked binds with no declared use.
//	redstone verify   the conformance runner: writes verdicts serve enforces.
//
// CLI — docker-style stack management, NO running redstone required (state is
// files; a running redstone live-reads whatever the CLI writes):
//
//	redstone validate  lint the whole registry (typos are errors, not silence)
//	redstone ls        list stacks, gives, uses
//	redstone add       register a stack file — validation-gated, atomic rollback
//	redstone rm        unregister a stack — refuses while others reference it
//
// State files:
//
//	<caps>/<name>.yaml    capability specs; level key order = the ladder
//	<dir>/stacks/         one stack per <name>.yaml or <name>/stack.yaml
//	<dir>/conformance.yaml generated verdicts (verify writes, serve reads)
//	<dir>/graph.jsonl     current wiring (auto-compacted; history in
//	<dir>/graph-archive.jsonl)
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// The banner — canonical copy in assets/banner.txt, reuse it everywhere.
const banner = `█▀▄ █▀▀ █▀▄ █▀▀ ▀█▀ █▀█ █▄ █ █▀▀
█▀▄ █▀▀ █ █ ▀▀█  █  █ █ █ ▀█ █▀▀
▀ ▀ ▀▀▀ ▀▀▀ ▀▀▀  ▀  ▀▀▀ ▀  ▀ ▀▀▀
●━━━●━━⚡━━●  the wiring layer  ●━━⚡━━●━━━●`

// Bumped by release-please on release PRs — never by hand.
const version = "0.1.0" // x-release-please-version

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, banner)
		fmt.Fprintln(os.Stderr, "\nusage: redstone <serve|verify|validate|ls|add|rm|version> [flags]")
		os.Exit(2)
	}

	registryFlags := func(fs *flag.FlagSet) (dir, caps *string) {
		dir = fs.String("registry", ".", "state dir (stacks/, conformance.yaml, graph.jsonl)")
		caps = fs.String("caps", "/capabilities", "capability specs dir")
		return
	}

	switch os.Args[1] {
	case "serve":
		fs := flag.NewFlagSet("serve", flag.ExitOnError)
		dir := fs.String("dir", ".", "state dir")
		caps := fs.String("caps", "/capabilities", "capability specs dir")
		httpAddr := fs.String("http", ":80", "HTTP listen address")
		grpcAddr := fs.String("grpc", ":50051", "gRPC listen address")
		strict := fs.Bool("strict", false, "refuse stacked binds that have no declared use")
		fs.Parse(os.Args[2:])
		k := &Kernel{Dir: *dir, Caps: *caps, Strict: *strict}

		if hasError := PrintIssues(k.Validate()); hasError {
			fmt.Println("⚠ powering on DEGRADED — broken circuits above (binds touching them will not power)")
		}
		warnStaleVerdicts(k)
		if kept, archived := k.CompactGraph(); archived > 0 {
			fmt.Printf("🧹 graph compacted at boot: %d kept, %d archived\n", kept, archived)
		}
		if *strict {
			fmt.Println("⚡ powering on (strict: undeclared circuits will not power)")
		} else {
			fmt.Println("⚡ powering on")
		}
		go serveGRPC(k, *grpcAddr)
		serveHTTP(k, *httpAddr) // blocks

	case "verify":
		fs := flag.NewFlagSet("verify", flag.ExitOnError)
		dir, caps := registryFlags(fs)
		fs.Parse(os.Args[2:])
		runVerify(&Kernel{Dir: *dir, Caps: *caps})

	case "validate":
		fs := flag.NewFlagSet("validate", flag.ExitOnError)
		dir, caps := registryFlags(fs)
		fs.Parse(os.Args[2:])
		if PrintIssues((&Kernel{Dir: *dir, Caps: *caps}).Validate()) {
			os.Exit(1)
		}

	case "ls":
		fs := flag.NewFlagSet("ls", flag.ExitOnError)
		dir, caps := registryFlags(fs)
		fs.Parse(os.Args[2:])
		k := &Kernel{Dir: *dir, Caps: *caps}
		stacks := k.loadStacks()
		fmt.Printf("⛏ %d stack(s) in this world\n", len(stacks))
		fmt.Printf("%-14s %-6s %-6s %s\n", "STACK", "GIVES", "USES", "DESCRIPTION")
		for _, s := range stacks {
			uses := 0
			for _, tasks := range s.File.Uses {
				uses += len(tasks)
			}
			fmt.Printf("%-14s %-6d %-6d %s\n", s.Name, len(s.File.Gives), uses, s.File.Description)
		}

	case "add":
		fs := flag.NewFlagSet("add", flag.ExitOnError)
		dir, caps := registryFlags(fs)
		name := fs.String("name", "", "stack name (default: source file basename)")
		force := fs.Bool("force", false, "overwrite an existing stack of the same name")
		fs.Parse(os.Args[2:])
		if fs.NArg() != 1 {
			fmt.Fprintln(os.Stderr, "usage: redstone add [flags] <stack.yaml>")
			os.Exit(2)
		}
		cliAdd(&Kernel{Dir: *dir, Caps: *caps}, fs.Arg(0), *name, *force)

	case "rm":
		fs := flag.NewFlagSet("rm", flag.ExitOnError)
		dir, caps := registryFlags(fs)
		force := fs.Bool("force", false, "remove even while other stacks reference its gives")
		fs.Parse(os.Args[2:])
		if fs.NArg() != 1 {
			fmt.Fprintln(os.Stderr, "usage: redstone rm [flags] <stack>")
			os.Exit(2)
		}
		cliRemove(&Kernel{Dir: *dir, Caps: *caps}, fs.Arg(0), *force)

	case "version":
		// Splash lines, menu-screen style. Humans get sparks; machines
		// should parse the first token only.
		splashes := []string{
			"verified before powered!",
			"never trust a claim you didn't test!",
			"one kernel to bind them all!",
			"0% cobblestone, 100% verdicts!",
			"the graph is the truth!",
			"friendship ended with hardcoded endpoints!",
		}
		fmt.Printf("redstone %s ⚡ %s\n", version, splashes[time.Now().UnixNano()%int64(len(splashes))])

	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		os.Exit(2)
	}
}

// cliAdd registers a stack file: write, validate the MERGED registry, keep
// only if clean — otherwise roll back exactly what was there before.
func cliAdd(k *Kernel, source, name string, force bool) {
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(source), ".yaml")
	}
	if !safeName.MatchString(name) {
		fmt.Fprintf(os.Stderr, "invalid stack name %q\n", name)
		os.Exit(1)
	}
	data, err := os.ReadFile(source)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	target := filepath.Join(k.Dir, "stacks", name+".yaml")
	previous, existed := os.ReadFile(target)
	if existed == nil && !force {
		fmt.Fprintf(os.Stderr, "stack %q already placed (use --force to replace)\n", name)
		os.Exit(1)
	}
	if err := os.WriteFile(target, data, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if hasError := PrintIssues(k.Validate()); hasError {
		if existed == nil {
			os.WriteFile(target, previous, 0o644)
		} else {
			os.Remove(target)
		}
		fmt.Fprintf(os.Stderr, "🧨 NOT placed — the creeper got it (registry rolled back). Fix the errors above.\n")
		os.Exit(1)
	}
	for _, s := range k.loadStacks() {
		if s.Name == name {
			uses := 0
			for _, tasks := range s.File.Uses {
				uses += len(tasks)
			}
			fmt.Printf("⛏ placed %q: %d gives, %d uses — a running redstone sees it next tick\n",
				name, len(s.File.Gives), uses)
		}
	}
}

// cliRemove unregisters a stack, refusing while other stacks' uses reference
// its gives (name-form) unless forced.
func cliRemove(k *Kernel, name string, force bool) {
	target := filepath.Join(k.Dir, "stacks", name+".yaml")
	if _, err := os.Stat(target); err != nil {
		if _, err := os.Stat(filepath.Join(k.Dir, "stacks", name, "stack.yaml")); err == nil {
			fmt.Fprintf(os.Stderr, "stack %q is folder-form — remove its directory yourself\n", name)
		} else {
			fmt.Fprintf(os.Stderr, "no stack %q\n", name)
		}
		os.Exit(1)
	}

	gives := map[string]bool{}
	for _, s := range k.loadStacks() {
		if s.Name == name {
			for g := range s.File.Gives {
				gives[g] = true
			}
		}
	}
	var refs []string
	var walk func(owner string, u Use)
	walk = func(owner string, u Use) {
		if u.Use != "" {
			if _, _, isCap := capabilityForm(u.Use); !isCap && gives[u.Use] {
				refs = append(refs, owner+" -> "+u.Use)
			}
		}
		for _, sub := range u.Wire {
			walk(owner, sub)
		}
	}
	for _, s := range k.loadStacks() {
		if s.Name == name {
			continue
		}
		for app, tasks := range s.File.Uses {
			for task, u := range tasks {
				walk(s.Name+"/"+app+"."+task, u)
			}
		}
	}
	if len(refs) > 0 && !force {
		sort.Strings(refs)
		fmt.Fprintf(os.Stderr, "NOT broken: %d circuit(s) still draw power from this stack's gives:\n  %s\n(--force to break anyway)\n",
			len(refs), strings.Join(refs, "\n  "))
		os.Exit(1)
	}
	if err := os.Remove(target); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("⛏ broke %q\n", name)
}

func warnStaleVerdicts(k *Kernel) {
	doc := k.VerdictsDocument()
	if doc.GeneratedAt == "" {
		fmt.Println("warn: no verdicts yet — run `redstone verify` (an untested circuit is just decoration)")
		return
	}
	if at, err := time.Parse("2006-01-02T15:04:05.000Z", doc.GeneratedAt); err == nil {
		if age := time.Since(at); age > 7*24*time.Hour {
			fmt.Printf("warn: verdicts are %.0f days old — re-run `redstone verify` (circuits corrode)\n", age.Hours()/24)
		}
	}
}
