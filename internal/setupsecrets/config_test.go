package setupsecrets

import (
	"bytes"
	"errors"
	"strings"
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
	var log bytes.Buffer
	config.fetch(&log)
	if _, ok := config.Sources["A"]; ok {
		t.Errorf("A is only needed by the host group, yet was kept:\n%s", log.String())
	}
	if v := config.Sources["B"].Value; v == nil || *v != "b" {
		t.Errorf("B: %v\n%s", v, log.String())
	}
	if err := config.SelectGroups([]string{"nope"}); err == nil || !strings.Contains(err.Error(), "known groups: cluster") {
		t.Errorf("unknown group: %v", err)
	}
}

func TestParseConfigAttempts(t *testing.T) {
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
	var log bytes.Buffer
	config.fetch(&log)
	config.store(&log)
	if !strings.Contains(log.String(), "D: a-none\n") {
		t.Errorf("log:\n%s", log.String())
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
	var log bytes.Buffer
	if got := config.fetch(&log); len(got) != 1 || got[0] != "BAD" {
		t.Errorf("fetch failures %v", got)
	}
	got := config.store(&log)
	if strings.Join(got, ",") != "needs bad,breaks" {
		t.Errorf("store failures %v\n%s", got, log.String())
	}
}
