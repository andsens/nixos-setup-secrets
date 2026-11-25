package setupsecrets

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func testConfig(t *testing.T, raw string) *Config {
	t.Helper()
	config, err := parseConfig([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return config
}

// auto runs setupAuto into a buffer
func auto(t *testing.T, config *Config, changedOnly bool) (string, error) {
	t.Helper()
	var log bytes.Buffer
	err := setupAuto(config, &syncWriter{w: &log}, changedOnly)
	return log.String(), err
}

func TestSelectGroups(t *testing.T) {
	config := testConfig(t, `{
		"sources": {"A": {"cmd": "echo a"}, "B": {"cmd": "echo b"}},
		"destinations": [
			{"logPrefix": "host dest", "group": "host", "requires": ["A"], "cmd": "true"},
			{"logPrefix": "cluster dest", "group": "cluster", "requires": ["B"], "cmd": "true"}
		]}`)
	if err := config.SelectGroups([]string{"cluster"}); err != nil {
		t.Fatal(err)
	}
	if len(config.Destinations) != 1 || config.Destinations[0].LogPrefix != "cluster dest" {
		t.Fatalf("kept %v", config.Destinations)
	}
	if len(config.all) != 2 {
		t.Errorf("all: %v", config.all)
	}
	log, err := auto(t, config, false)
	if err != nil {
		t.Fatal(err)
	}
	if config.Sources["A"].Value != nil || !strings.Contains(log, "A: Not needed by any destination that runs, omitting") {
		t.Errorf("A is only needed by the host group, yet was fetched:\n%s", log)
	}
	if v := config.Sources["B"].Value; v == nil || *v != "b" {
		t.Errorf("B: %v\n%s", v, log)
	}
	if err := config.SelectGroups([]string{"nope"}); err == nil || !strings.Contains(err.Error(), "known groups: cluster") {
		t.Errorf("unknown group: %v", err)
	}
}

func TestParseConfig(t *testing.T) {
	config := testConfig(t, `{"sources": {"A": {"cmd": "x", "timeout": "2m", "retries": 3}, "B": {"cmd": "x"}}, "destinations": []}`)
	if a := config.Sources["A"].Attempts; a.timeout != 2*time.Minute || a.Retries != 3 {
		t.Errorf("A: %+v", a)
	}
	if a := config.Sources["B"].Attempts; a.timeout != defaultTimeout {
		t.Errorf("B: %+v", a)
	}
	if _, err := parseConfig([]byte(`{"sources": {"A": {"timeout": "soon"}}}`)); err == nil {
		t.Error("a bad timeout parsed")
	}
	if _, err := parseConfig([]byte(`{"destinations": [{"logPrefix": "D", "outputs": ["nokind"]}]}`)); err == nil {
		t.Error("an output without a kind parsed")
	}
}

func noSleep(t *testing.T) *[]time.Duration {
	t.Helper()
	var slept []time.Duration
	sleep = func(d time.Duration) { slept = append(slept, d) }
	t.Cleanup(func() { sleep = time.Sleep })
	return &slept
}

func TestRunRetries(t *testing.T) {
	slept := noSleep(t)
	counter := t.TempDir() + "/n"
	// Fails twice, then prints the attempt count
	script := `n=$(( $(cat ` + counter + ` 2>/dev/null || echo 0) + 1 )); echo $n >` + counter + `; echo "try $n" >&2; [[ $n -ge 3 ]] && echo ok`
	var log bytes.Buffer
	out, err := run(&log, "SRC", script, nil, Attempts{Retries: 5, timeout: 5 * time.Second})
	if err != nil || strings.TrimSpace(string(out)) != "ok" {
		t.Fatalf("out %q, err %v\n%s", out, err, log.String())
	}
	if len(*slept) != 2 || (*slept)[0] != time.Second || (*slept)[1] != 2*time.Second {
		t.Errorf("backoff %v", *slept)
	}
	for _, want := range []string{"SRC: try 1\n", "SRC: Try 1 of 6 failed (exit status 1), next in 1s\n", "SRC: try 3\n"} {
		if !strings.Contains(log.String(), want) {
			t.Errorf("log lacks %q:\n%s", want, log.String())
		}
	}
}

func TestRunGivesUp(t *testing.T) {
	slept := noSleep(t)
	var log bytes.Buffer
	_, err := run(&log, "SRC", "exit 3", nil, Attempts{Retries: 2, timeout: 5 * time.Second})
	var exitErr interface{ ExitCode() int }
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 3 {
		t.Errorf("err %v", err)
	}
	if len(*slept) != 2 {
		t.Errorf("slept %v", *slept)
	}
}

func TestRunTimeout(t *testing.T) {
	var log bytes.Buffer
	start := time.Now()
	_, err := run(&log, "SRC", "sleep 30", nil, Attempts{timeout: 200 * time.Millisecond})
	if err == nil || !strings.Contains(err.Error(), "timed out after 200ms") {
		t.Errorf("err %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Errorf("took %s", time.Since(start))
	}
}

func TestPrefixWriter(t *testing.T) {
	var out bytes.Buffer
	p := &prefixWriter{w: &out, prefix: "P"}
	p.Write([]byte("one\ntw"))
	p.Write([]byte("o\nthree"))
	p.Flush()
	if out.String() != "P: one\nP: two\nP: three\n" {
		t.Errorf("%q", out.String())
	}
}

func TestStoreEnv(t *testing.T) {
	config := testConfig(t, `{
		"sources": {"A": {"cmd": "echo a"}, "W": {}},
		"destinations": [{"logPrefix": "D", "requires": ["A"], "wants": ["W"], "cmd": "echo \"$A-${W:-none}\" >&2"}]}`)
	log, err := auto(t, config, false)
	if err != nil || !strings.Contains(log, "D: a-none\n") {
		t.Errorf("err %v, log:\n%s", err, log)
	}
}

func TestFailures(t *testing.T) {
	noSleep(t)
	config := testConfig(t, `{
		"sources": {"OK": {"cmd": "echo ok"}, "BAD": {"cmd": "exit 1"}, "MANUAL": {}},
		"destinations": [
			{"logPrefix": "fine", "requires": ["OK"], "cmd": "true"},
			{"logPrefix": "needs bad", "requires": ["BAD"], "cmd": "true"},
			{"logPrefix": "needs manual", "requires": ["MANUAL"], "cmd": "true"},
			{"logPrefix": "breaks", "requires": ["OK"], "cmd": "exit 2"}
		]}`)
	log, err := auto(t, config, false)
	if err == nil || err.Error() != "Failed fetching BAD; storing breaks, needs bad" {
		t.Errorf("err %v\n%s", err, log)
	}
}

func TestFetchAndStoreOrder(t *testing.T) {
	noSleep(t)
	dir := t.TempDir()
	// A_WAITS sorts first and waits for a file the destination of QUICK makes, as an exec source waits for a workload
	// that waits for a Secret of the same run
	config := testConfig(t, `{
		"sources": {
			"A_WAITS": {"cmd": "for i in $(seq 100); do [[ -e `+dir+`/made ]] && echo ok && exit 0; sleep 0.05; done; exit 1"},
			"QUICK": {"cmd": "echo quick"},
			"EMPTY": {"cmd": "true"},
			"BROKEN": {"cmd": "exit 3"}
		},
		"destinations": [
			{"logPrefix": "uses the waiting one", "requires": ["A_WAITS"], "cmd": "true"},
			{"logPrefix": "makes the file", "requires": ["QUICK"], "cmd": "touch `+dir+`/made"},
			{"logPrefix": "nothing yet", "requires": ["EMPTY"], "cmd": "true"},
			{"logPrefix": "after a failure", "requires": ["BROKEN"], "cmd": "true"}
		]}`)
	var log bytes.Buffer
	fetchFailed, storeFailed, stored := config.fetchAndStore(&syncWriter{w: &log}, config.Destinations, 2)
	if strings.Join(fetchFailed, ",") != "BROKEN" {
		t.Errorf("fetch failures %v\n%s", fetchFailed, log.String())
	}
	// An empty value is "nothing yet", as for a secret without a command; a failed command is a failure
	if strings.Join(storeFailed, ",") != "after a failure" {
		t.Errorf("store failures %v\n%s", storeFailed, log.String())
	}
	if len(stored) != 2 || !strings.Contains(log.String(), "uses the waiting one: Storing") {
		t.Errorf("stored %v; the waiting source held up the destination it waits for:\n%s", stored, log.String())
	}
}

// stateConfig is a source whose value and token live in a file, read back by its destination, and an output of the
// built-in file kind
func stateConfig(t *testing.T, dir string, extra string) *Config {
	t.Helper()
	config := testConfig(t, `{
		"sources": {
			"STORED": {"cmd": "cat `+dir+`/value 2>/dev/null || echo first", "same": "stat -c %Y.%s `+dir+`/value 2>/dev/null || echo none"},
			"PLAIN": {"cmd": "echo plain"}
			`+extra+`
		},
		"destinations": [
			{"logPrefix": "stored", "requires": ["STORED"], "cmd": "printf %s \"$STORED\" >`+dir+`/value", "outputs": ["file:`+dir+`/value"]},
			{"logPrefix": "plain", "requires": ["PLAIN"], "cmd": "true"}
		],
		"outputKinds": {"file": {"exists": "test -e \"$OUTPUT\"", "delete": "rm -f -- \"$OUTPUT\""}}
	}`)
	config.statePath = filepath.Join(dir, "state", "state.json")
	return config
}

func TestChangedOnly(t *testing.T) {
	noSleep(t)
	dir := t.TempDir()
	log, err := auto(t, stateConfig(t, dir, ""), true)
	if err != nil || !strings.Contains(log, "stored: Runs, never stored") {
		t.Fatalf("first run: err %v\n%s", err, log)
	}
	state, err := readState(filepath.Join(dir, "state", "state.json"))
	if err != nil || state.Outputs["file:"+dir+"/value"].WrittenBy != "stored" {
		t.Fatalf("state: %v %+v", err, state)
	}

	// Nothing changed: the destination with a same command is skipped, the one without still runs
	log, err = auto(t, stateConfig(t, dir, ""), true)
	if err != nil || !strings.Contains(log, "stored: Unchanged") || strings.Contains(log, "STORED: Fetching") ||
		!strings.Contains(log, "plain: Runs, PLAIN has no same command") {
		t.Errorf("second run: err %v\n%s", err, log)
	}

	// The output is gone: the destination runs again
	os.Remove(filepath.Join(dir, "value"))
	log, _ = auto(t, stateConfig(t, dir, ""), true)
	if !strings.Contains(log, "stored: Runs, STORED changed") && !strings.Contains(log, "is gone") {
		t.Errorf("after the output went: %s", log)
	}

	// The token changed behind its back: it runs again
	time.Sleep(1100 * time.Millisecond)
	os.WriteFile(filepath.Join(dir, "value"), []byte("edited"), 0o600)
	log, _ = auto(t, stateConfig(t, dir, ""), true)
	if !strings.Contains(log, "stored: Runs, STORED changed") {
		t.Errorf("after a change: %s", log)
	}

	// Without --changed-only everything runs
	log, _ = auto(t, stateConfig(t, dir, ""), false)
	if !strings.Contains(log, "stored: Storing") {
		t.Errorf("without --changed-only: %s", log)
	}
}

func TestSameCantTell(t *testing.T) {
	noSleep(t)
	dir := t.TempDir()
	tries := filepath.Join(dir, "tries")
	// Counts its runs, and can't tell until the file "ready" exists; then the token is empty, a token like any other
	config := func() *Config {
		c := testConfig(t, `{
			"sources": {"LATER": {"cmd": "echo later", "same": "echo >>`+tries+`; [[ -e `+dir+`/ready ]] || exit 75"}},
			"destinations": [{"logPrefix": "later", "requires": ["LATER"], "cmd": "true"}]
		}`)
		c.statePath = filepath.Join(dir, "state.json")
		return c
	}
	if _, err := auto(t, config(), true); err != nil {
		t.Fatal(err)
	}
	log, err := auto(t, config(), true)
	if err != nil || !strings.Contains(log, "LATER: Same command can't tell yet") ||
		!strings.Contains(log, "later: Runs, LATER same command can't tell yet") || strings.Contains(log, "LATER: Try") {
		t.Errorf("can't tell: err %v\n%s", err, log)
	}
	state, _ := readState(filepath.Join(dir, "state.json"))
	if _, ok := state.Sources["LATER"]; ok {
		t.Errorf("a source that can't tell has a token in the state: %+v", state.Sources["LATER"])
	}
	// Each run plans and records once; no retries
	if raw, _ := os.ReadFile(tries); strings.Count(string(raw), "\n") != 4 {
		t.Errorf("same command ran %d times, want 4", strings.Count(string(raw), "\n"))
	}

	os.WriteFile(filepath.Join(dir, "ready"), nil, 0o600)
	log, _ = auto(t, config(), true)
	if !strings.Contains(log, "later: Runs, LATER changed") {
		t.Errorf("first token: %s", log)
	}
	log, _ = auto(t, config(), true)
	if !strings.Contains(log, "later: Unchanged") {
		t.Errorf("an empty token counts as changed: %s", log)
	}
}

func TestCleanup(t *testing.T) {
	noSleep(t)
	dir := t.TempDir()
	if _, err := auto(t, stateConfig(t, dir, ""), false); err != nil {
		t.Fatal(err)
	}
	// The destination is gone from the configuration, its output isn't
	config := stateConfig(t, dir, "")
	config.all = config.all[1:]
	var out, log bytes.Buffer
	if err := Cleanup(config, &out, &log, false); err != nil || !strings.Contains(out.String(), "file:"+dir+"/value") ||
		!strings.Contains(out.String(), "written by stored") {
		t.Fatalf("listing: %v\n%s", err, out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "value")); err != nil {
		t.Fatal("listing deleted the output")
	}
	out.Reset()
	if err := Cleanup(config, &out, &log, true); err != nil || !strings.Contains(out.String(), ": deleted") {
		t.Fatalf("deleting: %v\n%s", err, out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "value")); err == nil {
		t.Error("the output is still there")
	}
	out.Reset()
	if err := Cleanup(config, &out, &log, false); err != nil || !strings.Contains(out.String(), "still declared") {
		t.Errorf("after deleting: %v\n%s", err, out.String())
	}
}

func TestOutputExists(t *testing.T) {
	config := testConfig(t, `{"outputKinds": {"odd": {"exists": "exit 2"}, "file": {"exists": "test -e \"$OUTPUT\""}}}`)
	var log bytes.Buffer
	if ok, err := config.outputExists(&log, "file:/"); !ok || err != nil {
		t.Errorf("file:/ %v %v", ok, err)
	}
	if ok, err := config.outputExists(&log, "file:/nonexistent"); ok || err != nil {
		t.Errorf("file:/nonexistent %v %v", ok, err)
	}
	if _, err := config.outputExists(&log, "odd:x"); err == nil {
		t.Error("exit 2 isn't an answer")
	}
	if _, err := config.outputExists(&log, "unknown:x"); err == nil {
		t.Error("an unknown kind")
	}
}

// syncWriter makes a buffer safe for the concurrent fetches
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *syncWriter) Write(b []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(b)
}
