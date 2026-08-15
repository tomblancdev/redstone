package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Fixtures: a small fleet with a verified provider, a liar (claims mutable,
// verdict says core), and an app stack exercising every use-value form.
// All gives use check: none so tests never touch the network.
func fixtureKernel(t *testing.T) *Kernel {
	t.Helper()
	dir := t.TempDir()
	caps := filepath.Join(dir, "capabilities")
	stacks := filepath.Join(dir, "stacks")
	for _, d := range []string{caps, stacks} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	write := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write(filepath.Join(caps, "blob.yaml"), `
capability: blob
levels:
  core:
    operations: [put, get, resolve]
  mutable:
    extends: core
    operations: [update, delete, list]
`)

	write(filepath.Join(stacks, "infra.yaml"), `
version: 1
gives:
  good-blob:
    is: blob@mutable
    at: tcp://good
    check: none
  liar-blob:
    is: blob@mutable
    at: tcp://liar
    check: none
  idp:
    is: oidc@core
    at: tcp://idp
    check: none
  idp2: { is: oidc@core, at: tcp://idp2, check: none }
  idp3: { is: oidc@core, at: tcp://idp3, check: none }
  idp4: { is: oidc@core, at: tcp://idp4, check: none }
`)

	write(filepath.Join(stacks, "appstack.yaml"), `
version: 1
gives:
  own-blob:
    is: blob@core
    at: tcp://own
    check: none
uses:
  myapp:
    pinned: good-blob
    capform: blob@mutable
    wired:
      use: good-blob
      with: { space: t1 }
      wire:
        auth: idp
    cyclic:
      use: good-blob
      wire:
        a:
          use: idp
          wire:
            b: idp
`)

	write(filepath.Join(dir, "conformance.yaml"), `
results:
  good-blob: { capability: blob, claimed: mutable, verified_level: mutable, tests: [] }
  liar-blob: { capability: blob, claimed: mutable, verified_level: core, tests: [] }
`)

	return &Kernel{Dir: dir, Caps: caps}
}

func TestCapabilityForm(t *testing.T) {
	if c, l, ok := capabilityForm("blob@mutable"); !ok || c != "blob" || l != "mutable" {
		t.Fatalf("got %q %q %v", c, l, ok)
	}
	if _, _, ok := capabilityForm("give-name"); ok {
		t.Fatal("bare name must not parse as capability form")
	}
}

func TestUseUnmarshalForms(t *testing.T) {
	var u Use
	if err := yaml.Unmarshal([]byte(`blob@core`), &u); err != nil || u.Use != "blob@core" {
		t.Fatalf("scalar form: %v %+v", err, u)
	}
	var full Use
	err := yaml.Unmarshal([]byte(`{use: x, optional: true, with: {a: 1}, wire: {t: idp}}`), &full)
	if err != nil || full.Use != "x" || !full.Optional || full.Wire["t"].Use != "idp" {
		t.Fatalf("full form: %v %+v", err, full)
	}
}

func TestLadderOrderIsFileOrder(t *testing.T) {
	k := fixtureKernel(t)
	steps := k.Ladder("blob")
	if len(steps) != 2 || steps[0] != "core" || steps[1] != "mutable" {
		t.Fatalf("ladder %v", steps)
	}
}

func TestVerdictGateRefusesLiar(t *testing.T) {
	k := fixtureKernel(t)
	ok, refusal, bad := k.SelectInstance(BindParams{Capability: "blob", Level: "mutable", Name: "liar-blob", Consumer: "t", Stack: ""})
	if ok != nil || bad != "" || refusal == nil {
		t.Fatalf("expected refusal, got ok=%v bad=%q", ok, bad)
	}
	if !strings.Contains(refusal.Candidates[0].Reason, "verified only core") {
		t.Fatalf("reason %q", refusal.Candidates[0].Reason)
	}
}

func TestDeclarationDrivenBind(t *testing.T) {
	k := fixtureKernel(t)
	// No capability/level in the request: inherited from the name-form use.
	ok, refusal, bad := k.SelectInstance(BindParams{Stack: "appstack", Consumer: "myapp", As: "pinned"})
	if bad != "" || refusal != nil || ok == nil {
		t.Fatalf("bad=%q refusal=%+v", bad, refusal)
	}
	if ok.Name != "good-blob" || ok.Capability != "blob" || ok.RequestedLevel != "mutable" {
		t.Fatalf("got %+v", ok)
	}
}

func TestDriftRefused(t *testing.T) {
	k := fixtureKernel(t)
	_, _, bad := k.SelectInstance(BindParams{Capability: "oidc", Level: "core", Stack: "appstack", Consumer: "myapp", As: "capform"})
	if !strings.Contains(bad, "declares myapp.capform as blob") {
		t.Fatalf("bad=%q", bad)
	}
}

func TestUndeclaredWithoutCapabilityRefused(t *testing.T) {
	k := fixtureKernel(t)
	_, _, bad := k.SelectInstance(BindParams{Stack: "appstack", Consumer: "myapp", As: "mystery"})
	if !strings.Contains(bad, "no declared use") {
		t.Fatalf("bad=%q", bad)
	}
}

func TestSelfFirstScoping(t *testing.T) {
	k := fixtureKernel(t)
	// blob@core with stack context: the stack's own give wins over the
	// fleet's (higher-level) provider.
	ok, _, bad := k.SelectInstance(BindParams{Capability: "blob", Level: "core", Stack: "appstack", Consumer: "anyone", As: "x"})
	if bad != "" || ok == nil || ok.Name != "own-blob" {
		t.Fatalf("bad=%q ok=%+v", bad, ok)
	}
	// Without stack context, sorted-file fleet order applies instead.
	ok2, _, _ := k.SelectInstance(BindParams{Capability: "blob", Level: "core", Consumer: "anyone", As: "x"})
	if ok2 == nil || ok2.Name == "" {
		t.Fatal("fleet bind failed")
	}
}

func TestWireResolvedOntoEdge(t *testing.T) {
	k := fixtureKernel(t)
	ok, refusal, bad := k.SelectInstance(BindParams{Stack: "appstack", Consumer: "myapp", As: "wired"})
	if bad != "" || refusal != nil || ok == nil {
		t.Fatalf("bad=%q refusal=%+v", bad, refusal)
	}
	edge := k.EdgeFor("appstack", "myapp", "wired")
	if edge == nil || edge.With["space"] != "t1" {
		t.Fatalf("edge %+v", edge)
	}
	auth, _ := edge.Wire["auth"].(map[string]any)
	if auth == nil || auth["name"] != "idp" {
		t.Fatalf("wire %+v", edge.Wire)
	}
}

func TestWireCycleRefused(t *testing.T) {
	k := fixtureKernel(t)
	_, refusal, bad := k.SelectInstance(BindParams{Stack: "appstack", Consumer: "myapp", As: "cyclic"})
	if bad != "" || refusal == nil {
		t.Fatalf("expected refusal, bad=%q", bad)
	}
	if !strings.Contains(refusal.Candidates[len(refusal.Candidates)-1].Reason, "cycle") {
		t.Fatalf("reasons %+v", refusal.Candidates)
	}
}

func TestWireDepthRefused(t *testing.T) {
	k := fixtureKernel(t)
	// Deliberately invalid depth lives in its OWN stack file: the validator
	// (rightly) flags it, so it can't sit in the clean fixture.
	os.WriteFile(filepath.Join(k.Dir, "stacks", "deepstack.yaml"), []byte(`
version: 1
uses:
  dapp:
    deep:
      use: good-blob
      wire:
        l1: { use: idp, wire: { l2: { use: idp2, wire: { l3: { use: idp3, wire: { l4: idp4 } } } } } }
`), 0o644)
	_, refusal, bad := k.SelectInstance(BindParams{Stack: "deepstack", Consumer: "dapp", As: "deep"})
	if bad != "" || refusal == nil {
		t.Fatalf("expected refusal, bad=%q", bad)
	}
	joined := ""
	for _, c := range refusal.Candidates {
		joined += c.Reason + ";"
	}
	if !strings.Contains(joined, "depth") {
		t.Fatalf("reasons %q", joined)
	}
}

func TestGraphFoldAndCompaction(t *testing.T) {
	k := fixtureKernel(t)
	for i := 0; i < 5; i++ {
		k.appendEdge(Edge{Stack: "s", Consumer: "c", As: "t", Instance: "good-blob", At: "x"})
	}
	k.appendEdge(Edge{Stack: "s", Consumer: "c", As: "other", Instance: "good-blob", At: "x"})

	if got := len(k.boundEdges()); got != 2 {
		t.Fatalf("fold: got %d edges", got)
	}
	kept, archived := k.CompactGraph()
	if kept != 2 || archived != 4 {
		t.Fatalf("compact kept=%d archived=%d", kept, archived)
	}
	if got := len(k.boundEdges()); got != 2 {
		t.Fatalf("post-compact fold: %d", got)
	}
	if _, err := os.Stat(filepath.Join(k.Dir, "graph-archive.jsonl")); err != nil {
		t.Fatal("archive missing")
	}
	// Idempotent: nothing further to archive.
	if _, archived2 := k.CompactGraph(); archived2 != 0 {
		t.Fatalf("second compact archived %d", archived2)
	}
}

func hasIssue(issues []Issue, severity, fragment string) bool {
	for _, i := range issues {
		if i.Severity == severity && strings.Contains(i.Msg, fragment) {
			return true
		}
	}
	return false
}

func TestValidateFixtureHasNoErrors(t *testing.T) {
	k := fixtureKernel(t)
	for _, i := range k.Validate() {
		if i.Severity == "error" {
			t.Fatalf("unexpected error: %+v", i)
		}
	}
	// oidc has no spec in the fixture: claimed trust must be a WARNING.
	if !hasIssue(k.Validate(), "warn", "no spec for capability") {
		t.Fatal("expected claimed-trust warning for oidc")
	}
}

func TestValidateCatchesTypoLevel(t *testing.T) {
	k := fixtureKernel(t)
	os.WriteFile(filepath.Join(k.Dir, "stacks", "typo.yaml"), []byte(`
version: 1
gives:
  broken: { is: blob@mutble, at: tcp://x, check: none }
`), 0o644)
	if !hasIssue(k.Validate(), "error", "can NEVER bind") {
		t.Fatalf("typo level not caught: %+v", k.Validate())
	}
}

func TestValidateCatchesUnknownField(t *testing.T) {
	k := fixtureKernel(t)
	os.WriteFile(filepath.Join(k.Dir, "stacks", "field.yaml"), []byte(`
version: 1
gives:
  x: { is: blob@core, at: tcp://x, check: none }
uses:
  app:
    t: { use: x, wih: { a: 1 } }
`), 0o644)
	if !hasIssue(k.Validate(), "error", "invalid stack file") {
		t.Fatalf("unknown field not caught: %+v", k.Validate())
	}
}

func TestValidateCatchesGhostReference(t *testing.T) {
	k := fixtureKernel(t)
	os.WriteFile(filepath.Join(k.Dir, "stacks", "ghost.yaml"), []byte(`
version: 1
uses:
  app:
    t: no-such-give
`), 0o644)
	if !hasIssue(k.Validate(), "error", "no stack provides") {
		t.Fatalf("ghost reference not caught: %+v", k.Validate())
	}
}

func TestStrictRefusesUndeclaredStackedBind(t *testing.T) {
	k := fixtureKernel(t)
	k.Strict = true
	_, _, bad := k.SelectInstance(BindParams{Capability: "blob", Level: "core", Stack: "appstack", Consumer: "myapp", As: "freestyle"})
	if !strings.Contains(bad, "strict mode") {
		t.Fatalf("bad=%q", bad)
	}
	// Declared binds still work, and unstacked (legacy) binds are untouched.
	if ok, _, bad := k.SelectInstance(BindParams{Stack: "appstack", Consumer: "myapp", As: "pinned"}); ok == nil || bad != "" {
		t.Fatalf("declared bind broke under strict: %q", bad)
	}
	if ok, _, _ := k.SelectInstance(BindParams{Capability: "blob", Level: "core", Consumer: "legacy", As: "x"}); ok == nil {
		t.Fatal("unstacked bind broke under strict")
	}
}

func TestStackNameSanitized(t *testing.T) {
	k := fixtureKernel(t)
	if k.usesFor("../../etc") != nil || k.usesFor("defaults") == nil && k.usesFor("../x") != nil {
		t.Fatal("path-shaped stack names must be rejected")
	}
}
