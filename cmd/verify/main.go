// verify boots the server and probes every public surface.
// Outputs a JSON report and a final score out of 10.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type check struct {
	Name    string  `json:"name"`
	Weight  float64 `json:"weight"`
	Passed  bool    `json:"passed"`
	Detail  string  `json:"detail,omitempty"`
	Latency string  `json:"latency,omitempty"`
}

type report struct {
	Checks []check `json:"checks"`
	Score  float64 `json:"score"`
	Max    float64 `json:"max"`
	OutOf  int     `json:"out_of"`
}

func main() {
	r := &report{Max: 10}

	r.add(runStaticBuild())
	r.add(runUnitTests())

	port, server, err := bootServer()
	if err != nil {
		r.add(check{Name: "server.boot", Weight: 7, Detail: "server failed to boot: " + err.Error()})
		r.finalize()
		return
	}
	defer killServer(server)

	base := "http://127.0.0.1:" + port

	r.add(probeHealth(base))
	r.add(probeJSON(base+"/api/events", "events.api", 0.5, "events"))
	r.add(probeJSON(base+"/api/chain", "chain.config", 0.5, "chain_id"))
	r.add(probeJSON(base+"/api/dev/leaderboard", "dev.leaderboard.api", 0.5, "leaderboard"))
	r.add(probeJSON(base+"/api/dev/0x0000000000000000000000000000000000000001", "dev.profile.api", 0.5, "address"))
	r.add(probeContent(base+"/api/dev/0x0000000000000000000000000000000000000001/badge.svg", "dev.badge", 0.5, "image/svg+xml", "Signet"))
	r.add(probeContent(base+"/api/dev/0x0000000000000000000000000000000000000001/og.svg", "dev.og", 0.25, "image/svg+xml", "SIGNET"))
	r.add(probeJSON(base+"/api/dev/0x0000000000000000000000000000000000000001/timeseries", "dev.timeseries", 0.25, "timeseries"))
	r.add(probeJSON(base+"/api/dev/recent", "dev.recent", 0.25, "recent"))
	r.add(probeContent(base+"/embed/0x0000000000000000000000000000000000000001?v=detailed", "embed.detailed", 0.25, "text/html", "SIGNET"))
	r.add(probeContent(base+"/embed/0x0000000000000000000000000000000000000001", "embed.widget", 0.5, "text/html", "SIGNET"))
	r.add(probeContent(base+"/dev/leaderboard", "dev.leaderboard.page", 0.5, "text/html", "SIGNET"))
	r.add(probeContent(base+"/dev", "dev.home", 0.25, "text/html", "Proof of code"))
	r.add(probeRedirect(base+"/web3", "/dev/leaderboard"))
	r.add(probeHookRoundtrip(base))
	r.add(probeAIScoring(base))
	r.add(probeLiveSocket(base))
	r.add(probeStatusCode(base+"/api/proof/nonexistent", "proof.404", 0.5, http.StatusNotFound))
	r.add(probeStatusCode(base+"/", "static.root", 0.5, http.StatusOK))

	r.finalize()
}

func (r *report) add(c check) {
	r.Checks = append(r.Checks, c)
}

func (r *report) finalize() {
	var got float64
	for _, c := range r.Checks {
		if c.Passed {
			got += c.Weight
		}
	}
	r.Score = round1(got)
	if r.Score > r.Max {
		r.Score = r.Max
	}
	r.OutOf = 10

	out, _ := json.MarshalIndent(r, "", "  ")
	fmt.Println(string(out))

	fmt.Println()
	fmt.Println("RUBRIC")
	fmt.Println("------")
	for _, c := range r.Checks {
		mark := "FAIL"
		if c.Passed {
			mark = "PASS"
		}
		fmt.Printf("[%s] %-28s  %.1f pts  %s\n", mark, c.Name, c.Weight, c.Detail)
	}
	fmt.Println()
	fmt.Printf("SCORE: %.1f / %d\n", r.Score, r.OutOf)
}

func round1(f float64) float64 {
	return float64(int(f*10+0.5)) / 10
}

func runStaticBuild() check {
	c := check{Name: "go.build", Weight: 1.5}
	start := time.Now()
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	c.Latency = time.Since(start).String()
	if err != nil {
		c.Detail = "build failed: " + strings.TrimSpace(string(out))
		return c
	}
	c.Passed = true
	c.Detail = "clean"
	return c
}

func runUnitTests() check {
	c := check{Name: "go.test", Weight: 1.0}
	start := time.Now()
	cmd := exec.Command("go", "test", "-count=1", "./...")
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	c.Latency = time.Since(start).String()
	body := strings.TrimSpace(string(out))
	if err != nil {
		c.Detail = "tests failed: " + tail(body, 400)
		return c
	}
	c.Passed = true
	c.Detail = "all packages clean"
	return c
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}

func freePort() (string, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer l.Close()
	addr := l.Addr().(*net.TCPAddr)
	return strconv.Itoa(addr.Port), nil
}

func bootServer() (string, *exec.Cmd, error) {
	port, err := freePort()
	if err != nil {
		return "", nil, err
	}

	cmd := exec.Command("go", "run", "./cmd/server")
	cmd.Env = append(os.Environ(),
		"PORT="+port,
		"DATABASE_URL=", // force memory store
		"REDIS_URL=",    // force in-memory relay
		"HOOK_TOKEN=verify-token",
		"SIGNET_DEV_SEED=1",
	)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		return "", nil, err
	}

	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://127.0.0.1:" + port + "/health")
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			return port, cmd, nil
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(250 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
	return "", nil, fmt.Errorf("server never reached /health on :%s", port)
}

func killServer(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
	_, _ = cmd.Process.Wait()
}

func probeHealth(base string) check {
	return probeJSON(base+"/health", "health", 0.5, "status")
}

func probeJSON(target, name string, weight float64, mustKey string) check {
	c := check{Name: name, Weight: weight}
	start := time.Now()
	resp, err := http.Get(target)
	c.Latency = time.Since(start).String()
	if err != nil {
		c.Detail = err.Error()
		return c
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		c.Detail = fmt.Sprintf("status %d: %s", resp.StatusCode, tail(string(body), 200))
		return c
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		c.Detail = "non-json body: " + tail(string(body), 200)
		return c
	}
	if _, ok := raw[mustKey]; !ok && mustKey != "" {
		c.Detail = "missing key: " + mustKey
		return c
	}
	c.Passed = true
	c.Detail = fmt.Sprintf("200 ok (%d bytes)", len(body))
	return c
}

func probeContent(target, name string, weight float64, mustContentType, mustContain string) check {
	c := check{Name: name, Weight: weight}
	start := time.Now()
	resp, err := http.Get(target)
	c.Latency = time.Since(start).String()
	if err != nil {
		c.Detail = err.Error()
		return c
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		c.Detail = fmt.Sprintf("status %d", resp.StatusCode)
		return c
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), mustContentType) {
		c.Detail = "wrong content-type: " + resp.Header.Get("Content-Type")
		return c
	}
	if !strings.Contains(string(body), mustContain) {
		c.Detail = "missing substring: " + mustContain
		return c
	}
	c.Passed = true
	c.Detail = fmt.Sprintf("200 ok %s (%d bytes)", resp.Header.Get("Content-Type"), len(body))
	return c
}

func probeStatusCode(target, name string, weight float64, want int) check {
	c := check{Name: name, Weight: weight}
	resp, err := http.Get(target)
	if err != nil {
		c.Detail = err.Error()
		return c
	}
	defer resp.Body.Close()
	if resp.StatusCode != want {
		c.Detail = fmt.Sprintf("got %d, want %d", resp.StatusCode, want)
		return c
	}
	c.Passed = true
	c.Detail = fmt.Sprintf("expected %d", want)
	return c
}

func probeRedirect(target, expectLocationContains string) check {
	c := check{Name: "web3.redirect", Weight: 0.5}
	cli := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := cli.Get(target)
	if err != nil {
		c.Detail = err.Error()
		return c
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound && resp.StatusCode != http.StatusMovedPermanently && resp.StatusCode != http.StatusSeeOther {
		c.Detail = fmt.Sprintf("status %d (not a redirect)", resp.StatusCode)
		return c
	}
	loc, _ := resp.Location()
	if loc == nil || !strings.Contains(loc.Path, expectLocationContains) {
		got := ""
		if loc != nil {
			got = loc.String()
		}
		c.Detail = "redirect to " + got
		return c
	}
	c.Passed = true
	c.Detail = fmt.Sprintf("%d -> %s", resp.StatusCode, loc.Path)
	return c
}

func probeHookRoundtrip(base string) check {
	c := check{Name: "hook.roundtrip", Weight: 1.0}
	body := map[string]any{
		"type":           "push",
		"repo":           "verify/repo",
		"ref":            "refs/heads/main",
		"branch":         "main",
		"old_sha":        "0000000000000000000000000000000000000000",
		"new_sha":        "abcdefabcdefabcdefabcdefabcdefabcdefabcd",
		"pusher":         "verifier",
		"commits_detail": []map[string]string{{"sha": "abcd", "message": "verify probe"}},
	}
	bs, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", base+"/hook", bytes.NewReader(bs))
	req.Header.Set("Authorization", "Bearer verify-token")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.Detail = err.Error()
		return c
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		out, _ := io.ReadAll(resp.Body)
		c.Detail = fmt.Sprintf("status %d: %s", resp.StatusCode, tail(string(out), 200))
		return c
	}

	// Now confirm event flowed into recent events.
	time.Sleep(400 * time.Millisecond)
	r2, err := http.Get(base + "/api/events?limit=10")
	if err != nil {
		c.Detail = "events fetch: " + err.Error()
		return c
	}
	defer r2.Body.Close()
	bb, _ := io.ReadAll(r2.Body)
	if !strings.Contains(string(bb), "verify/repo") {
		c.Detail = "ingested event not visible in /api/events"
		return c
	}
	c.Passed = true
	c.Detail = "POST /hook -> event visible in /api/events"
	return c
}

// probeAIScoring posts another push and waits for the async heuristic reviewer
// to populate quality_score on the event.
func probeAIScoring(base string) check {
	c := check{Name: "ai.scoring", Weight: 0.5}

	body := map[string]any{
		"type":           "push",
		"repo":           "verify/scoring",
		"ref":            "refs/heads/main",
		"branch":         "main",
		"old_sha":        "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
		"new_sha":        "1234567890abcdef1234567890abcdef12345678",
		"pusher":         "verifier",
		"commits_detail": []map[string]string{{"sha": "1234", "message": "add tests for fix"}},
	}
	bs, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", base+"/hook", bytes.NewReader(bs))
	req.Header.Set("Authorization", "Bearer verify-token")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.Detail = "post: " + err.Error()
		return c
	}
	resp.Body.Close()

	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(400 * time.Millisecond)
		r2, err := http.Get(base + "/api/events?limit=20")
		if err != nil {
			continue
		}
		bb, _ := io.ReadAll(r2.Body)
		r2.Body.Close()

		var parsed struct {
			Events []map[string]any `json:"events"`
		}
		if err := json.Unmarshal(bb, &parsed); err != nil {
			continue
		}
		for _, e := range parsed.Events {
			if e["repo"] != "verify/scoring" {
				continue
			}
			score, _ := e["quality_score"].(float64)
			cat, _ := e["category"].(string)
			if score > 0 && cat != "" {
				c.Passed = true
				c.Detail = fmt.Sprintf("heuristic scored %.0f category=%s", score, cat)
				return c
			}
		}
	}
	c.Detail = "no quality_score populated within 8s"
	return c
}

func probeLiveSocket(base string) check {
	c := check{Name: "live.websocket", Weight: 0.5}
	u, _ := url.Parse(base + "/live")
	conn, err := net.DialTimeout("tcp", u.Host, 2*time.Second)
	if err != nil {
		c.Detail = err.Error()
		return c
	}
	defer conn.Close()
	key := "dGhlIHNhbXBsZSBub25jZQ==" // dummy 16-byte base64 nonce
	req := "GET /live HTTP/1.1\r\n" +
		"Host: " + u.Host + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := conn.Write([]byte(req)); err != nil {
		c.Detail = err.Error()
		return c
	}
	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err != nil {
		c.Detail = "ws read: " + err.Error()
		return c
	}
	if !strings.Contains(string(buf[:n]), "101") {
		c.Detail = "no 101 switching protocols: " + tail(string(buf[:n]), 120)
		return c
	}
	c.Passed = true
	c.Detail = "101 Switching Protocols"
	return c
}
