package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// The conformance runner — tests every instance against the executable
// contract of its capability and records the highest level it actually
// passes. A REPORTER, not a gate: always exits 0; enforcement is the
// register's job. Test names are stable identifiers — keep them, so verdict
// files stay diffable across versions.

var verifyClient = http.Client{Timeout: 10 * time.Second}

func randID() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 8)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}

type tester struct{ tests []TestResult }

func (t *tester) expect(name string, fn func() error) bool {
	if err := fn(); err != nil {
		t.tests = append(t.tests, TestResult{Name: name, Ok: false, Error: err.Error()})
		return false
	}
	t.tests = append(t.tests, TestResult{Name: name, Ok: true})
	return true
}

type jsonResp struct {
	Status int
	Body   map[string]any
	Raw    []byte
}

func request(method, url, contentType string, body []byte) (jsonResp, error) {
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		return jsonResp{}, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := verifyClient.Do(req)
	if err != nil {
		return jsonResp{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	out := jsonResp{Status: resp.StatusCode, Raw: raw}
	json.Unmarshal(raw, &out.Body)
	return out, nil
}

func fetchText(url string) (int, string, error) {
	resp, err := verifyClient.Get(url)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw), nil
}

/* ---------------- blob pack ---------------- */

func blobCore(endpoint string, t *tester) {
	var flags map[string]any
	t.expect("manifest", func() error {
		r, err := request("GET", endpoint+"/manifest", "", nil)
		if err != nil {
			return err
		}
		if r.Status != 200 {
			return fmt.Errorf("manifest %d", r.Status)
		}
		if r.Body["capability"] != "blob" {
			return fmt.Errorf("capability=%v", r.Body["capability"])
		}
		f, ok := r.Body["flags"].(map[string]any)
		if !ok {
			return fmt.Errorf("flags missing")
		}
		if _, ok := f["direct_read"].(bool); !ok {
			return fmt.Errorf("flags.direct_read missing")
		}
		flags = f
		return nil
	})

	payload := "conformance " + randID()
	var id string
	t.expect("put", func() error {
		r, err := request("POST", endpoint+"/objects", "text/plain", []byte(payload))
		if err != nil {
			return err
		}
		if r.Status != 201 {
			return fmt.Errorf("put %d", r.Status)
		}
		got, _ := r.Body["id"].(string)
		if got == "" {
			return fmt.Errorf("no id")
		}
		if size, ok := r.Body["size"].(float64); !ok || int(size) != len(payload) {
			return fmt.Errorf("size %v", r.Body["size"])
		}
		id = got
		return nil
	})
	if id == "" {
		return
	}

	t.expect("resolve+direct-get", func() error {
		r, err := request("GET", endpoint+"/objects/"+id+"/resolve?mode=read&audience=internal", "", nil)
		if err != nil {
			return err
		}
		if r.Status != 200 {
			return fmt.Errorf("resolve %d", r.Status)
		}
		if r.Body["method"] != "GET" {
			return fmt.Errorf("method %v", r.Body["method"])
		}
		if _, present := r.Body["expires_at"]; !present {
			return fmt.Errorf("expires_at missing")
		}
		status, text, err := fetchText(r.Body["url"].(string))
		if err != nil {
			return err
		}
		if status != 200 {
			return fmt.Errorf("direct fetch %d", status)
		}
		if text != payload {
			return fmt.Errorf("bytes differ")
		}
		return nil
	})

	t.expect("resolve-public-shape", func() error {
		r, err := request("GET", endpoint+"/objects/"+id+"/resolve?mode=read", "", nil)
		if err != nil {
			return err
		}
		url, _ := r.Body["url"].(string)
		if r.Status != 200 || !strings.HasPrefix(url, "http") {
			return fmt.Errorf("no public url")
		}
		return nil
	})

	t.expect("flags-honesty:direct_write", func() error {
		r, err := request("GET", endpoint+"/objects/"+id+"/resolve?mode=write&audience=internal", "", nil)
		if err != nil {
			return err
		}
		if directWrite, _ := flags["direct_write"].(bool); directWrite {
			if r.Status != 200 {
				return fmt.Errorf("flag says direct_write yet resolve write %d", r.Status)
			}
			method, _ := r.Body["method"].(string)
			put, err := request(method, r.Body["url"].(string), "", []byte("direct-write "+randID()))
			if err != nil {
				return err
			}
			if put.Status >= 300 {
				return fmt.Errorf("presigned PUT %d", put.Status)
			}
		} else if r.Status < 400 {
			return fmt.Errorf("flag says no direct_write yet resolve write returned %d", r.Status)
		}
		return nil
	})
}

func blobMutable(endpoint string, t *tester) {
	key := "conf-" + randID()

	t.expect("update(create-at-key)", func() error {
		r, err := request("PUT", endpoint+"/objects/"+key, "text/plain", []byte("v1"))
		if err != nil {
			return err
		}
		if r.Status != 200 {
			return fmt.Errorf("PUT %d", r.Status)
		}
		return nil
	})

	t.expect("update(overwrite)", func() error {
		if _, err := request("PUT", endpoint+"/objects/"+key, "text/plain", []byte("v2")); err != nil {
			return err
		}
		r, err := request("GET", endpoint+"/objects/"+key+"/resolve?mode=read&audience=internal", "", nil)
		if err != nil {
			return err
		}
		url, _ := r.Body["url"].(string)
		if url == "" {
			return fmt.Errorf("resolve %d", r.Status)
		}
		_, text, err := fetchText(url)
		if err != nil {
			return err
		}
		if text != "v2" {
			return fmt.Errorf("overwrite not visible")
		}
		return nil
	})

	t.expect("list", func() error {
		r, err := request("GET", endpoint+"/objects", "", nil)
		if err != nil {
			return err
		}
		if r.Status != 200 {
			return fmt.Errorf("list %d", r.Status)
		}
		objects, _ := r.Body["objects"].([]any)
		for _, o := range objects {
			if m, ok := o.(map[string]any); ok && m["id"] == key {
				return nil
			}
		}
		return fmt.Errorf("listed object missing")
	})

	t.expect("delete", func() error {
		r, err := request("DELETE", endpoint+"/objects/"+key, "", nil)
		if err != nil {
			return err
		}
		if r.Status >= 300 {
			return fmt.Errorf("DELETE %d", r.Status)
		}
		res, err := request("GET", endpoint+"/objects/"+key+"/resolve?mode=read&audience=internal", "", nil)
		if err != nil {
			return err
		}
		url, _ := res.Body["url"].(string)
		if url == "" {
			return nil // resolve itself refusing a deleted id is acceptable
		}
		status, _, err := fetchText(url)
		if err != nil {
			return err
		}
		if status < 400 {
			return fmt.Errorf("deleted object still readable (%d)", status)
		}
		return nil
	})
}

/* ---------------- mail / secrets packs ---------------- */

func mailCore(endpoint string, t *tester) {
	t.expect("manifest", func() error {
		r, err := request("GET", endpoint+"/manifest", "", nil)
		if err != nil {
			return err
		}
		if r.Status != 200 || r.Body["capability"] != "mail" {
			return fmt.Errorf("bad manifest")
		}
		return nil
	})
	t.expect("send", func() error {
		body, _ := json.Marshal(map[string]string{
			"to": "conformance@example.test", "subject": "conformance", "text": "probe " + randID(),
		})
		r, err := request("POST", endpoint+"/send", "application/json", body)
		if err != nil {
			return err
		}
		if r.Status != 200 {
			return fmt.Errorf("send %d", r.Status)
		}
		return nil
	})
	t.expect("send-rejects-malformed", func() error {
		r, err := request("POST", endpoint+"/send", "application/json", []byte("{}"))
		if err != nil {
			return err
		}
		if r.Status < 400 {
			return fmt.Errorf("accepted a message with no recipient (%d)", r.Status)
		}
		return nil
	})
}

func secretsCore(endpoint string, t *tester) {
	t.expect("manifest", func() error {
		r, err := request("GET", endpoint+"/manifest", "", nil)
		if err != nil {
			return err
		}
		if r.Status != 200 || r.Body["capability"] != "secrets" {
			return fmt.Errorf("bad manifest")
		}
		return nil
	})
	probe := randID()
	t.expect("put+get", func() error {
		body, _ := json.Marshal(map[string]string{"probe": probe})
		put, err := request("PUT", endpoint+"/secrets/conformance-probe", "application/json", body)
		if err != nil {
			return err
		}
		if put.Status != 200 {
			return fmt.Errorf("put %d", put.Status)
		}
		got, err := request("GET", endpoint+"/secrets/conformance-probe", "", nil)
		if err != nil {
			return err
		}
		if got.Status != 200 || got.Body["probe"] != probe {
			return fmt.Errorf("roundtrip failed")
		}
		return nil
	})
	t.expect("get-missing-is-404", func() error {
		r, err := request("GET", endpoint+"/secrets/does-not-exist-"+randID(), "", nil)
		if err != nil {
			return err
		}
		if r.Status != 404 {
			return fmt.Errorf("expected 404, got %d", r.Status)
		}
		return nil
	})
}

/* ---------------- core capability packs (the kernel verifying itself) --- */

func manifestCheck(endpoint, capability string, t *tester) {
	t.expect("manifest", func() error {
		r, err := request("GET", endpoint+"/manifest", "", nil)
		if err != nil {
			return err
		}
		if r.Status != 200 || r.Body["capability"] != capability {
			return fmt.Errorf("bad manifest (%d, capability=%v)", r.Status, r.Body["capability"])
		}
		return nil
	})
}

func registryCore(endpoint string, t *tester) {
	manifestCheck(endpoint, "registry", t)
	t.expect("bind-requires-params", func() error {
		r, err := request("GET", endpoint+"/bind", "", nil)
		if err != nil {
			return err
		}
		if r.Status != 400 {
			return fmt.Errorf("bare bind returned %d, want 400", r.Status)
		}
		return nil
	})
	t.expect("instances", func() error {
		r, err := request("GET", endpoint+"/instances", "", nil)
		if err != nil {
			return err
		}
		if r.Status != 200 {
			return fmt.Errorf("instances %d", r.Status)
		}
		if _, ok := r.Body["instances"].([]any); !ok {
			return fmt.Errorf("instances array missing")
		}
		return nil
	})
}

func graphCore(endpoint string, t *tester) {
	manifestCheck(endpoint, "graph", t)
	t.expect("edges", func() error {
		r, err := request("GET", endpoint+"/edges", "", nil)
		if err != nil {
			return err
		}
		if r.Status != 200 {
			return fmt.Errorf("edges %d", r.Status)
		}
		if _, ok := r.Body["edges"].([]any); !ok {
			return fmt.Errorf("edges array missing")
		}
		return nil
	})
}

func conformanceCore(endpoint string, t *tester) {
	manifestCheck(endpoint, "conformance", t)
	t.expect("verdicts", func() error {
		r, err := request("GET", endpoint+"/verdicts", "", nil)
		if err != nil {
			return err
		}
		if r.Status != 200 {
			return fmt.Errorf("verdicts %d", r.Status)
		}
		if _, ok := r.Body["results"].(map[string]any); !ok {
			return fmt.Errorf("results map missing")
		}
		return nil
	})
}

/* ---------------- driver ---------------- */

var packs = map[string]map[string]func(string, *tester){
	"blob":    {"core": blobCore, "mutable": blobMutable},
	"mail":    {"core": mailCore},
	"secrets": {"core": secretsCore},
	// The kernel's own services, verified with the same machinery:
	"registry":    {"core": registryCore},
	"graph":       {"core": graphCore},
	"conformance": {"core": conformanceCore},
}

func runVerify(k *Kernel) {
	results := map[string]Verdict{}

	for _, inst := range k.loadInstances() {
		pack, hasPack := packs[inst.Capability]
		if !hasPack {
			continue // no pack: the register falls back to claimed trust
		}
		steps := k.Ladder(inst.Capability)
		claimedRank := rank(steps, inst.Level)

		t := &tester{}
		var verified *string
		for i := 0; i <= claimedRank && i < len(steps); i++ {
			level := steps[i]
			fn, ok := pack[level]
			if !ok {
				// A level with no test pack is UNVERIFIABLE, never auto-passed.
				t.tests = append(t.tests, TestResult{Name: level, Ok: false, Error: "no conformance pack for this level"})
				break
			}
			before := len(t.tests)
			fn(inst.Endpoint, t)
			failed := false
			for _, tr := range t.tests[before:] {
				if !tr.Ok {
					failed = true
				}
			}
			if failed {
				break // a level must pass in full before the next runs
			}
			levelCopy := level
			verified = &levelCopy
		}

		results[inst.Name] = Verdict{
			Capability: inst.Capability, Claimed: inst.Level,
			VerifiedLevel: verified, Tests: t.tests,
		}

		// Tokens (claimed / OK / DOWNGRADED / FAIL) are stable for grepping;
		// only the spark is decoration.
		verdict, spark := "OK", "⚡"
		if verified == nil {
			verdict, spark = "DOWNGRADED to nothing", "🔌"
		} else if *verified != inst.Level {
			verdict, spark = "DOWNGRADED to "+*verified, "🔌"
		}
		fmt.Printf("%s %s: claimed %s@%s -> %s\n", spark, inst.Name, inst.Capability, inst.Level, verdict)
		for _, tr := range t.tests {
			if !tr.Ok {
				fmt.Printf("   FAIL %s: %s\n", tr.Name, tr.Error)
			}
		}
	}

	out, err := yaml.Marshal(conformanceFile{
		GeneratedAt: time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		Results:     results,
	})
	if err != nil {
		fmt.Println("marshal:", err)
		return
	}
	header := []byte("# Generated by redstone verify — do not edit. The register enforces this.\n")

	// Write-then-rename: the register re-reads this file on every request.
	tmp := filepath.Join(k.Dir, "conformance.yaml.tmp")
	final := filepath.Join(k.Dir, "conformance.yaml")
	if err := os.WriteFile(tmp, append(header, out...), 0o644); err != nil {
		fmt.Println("write:", err)
		return
	}
	if err := os.Rename(tmp, final); err != nil {
		fmt.Println("rename:", err)
		return
	}
	fmt.Println("⚡ wrote conformance.yaml — the verdicts are law")
}
