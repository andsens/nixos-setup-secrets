package setupsecrets

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

const maxBackoff = 30 * time.Second

// sleep waits between attempts, a variable so the tests don't have to
var sleep = time.Sleep

// run runs script with bash and the given extra environment, prefixing each line it prints on stderr with prefix.
// A failed attempt, a timeout included, is retried attempts.Retries times, after 1s, 2s, 4s and so on up to 30s.
func run(log io.Writer, prefix, script string, env []string, attempts Attempts) ([]byte, error) {
	backoff := time.Second
	for attempt := 0; ; attempt++ {
		out, err := runOnce(log, prefix, script, env, attempts.timeout)
		if err == nil || attempt >= attempts.Retries {
			return out, err
		}
		fmt.Fprintf(log, "%s: Try %d of %d failed (%s), next in %s\n", prefix, attempt+1, attempts.Retries+1, err, backoff)
		sleep(backoff)
		backoff = min(2*backoff, maxBackoff)
	}
}

func runOnce(log io.Writer, prefix, script string, env []string, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-c", script)
	cmd.WaitDelay = time.Second
	cmd.Env = append(os.Environ(), env...)
	stderr := &prefixWriter{w: log, prefix: prefix}
	cmd.Stderr = stderr
	out, err := cmd.Output()
	stderr.Flush()
	if ctx.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("timed out after %s", timeout)
	}
	return out, err
}

// prefixWriter writes each complete line it is given to w, prefixed with "prefix: "
type prefixWriter struct {
	w      io.Writer
	prefix string
	mu     sync.Mutex
	buf    bytes.Buffer
}

func (p *prefixWriter) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.buf.Write(b)
	for {
		line, err := p.buf.ReadBytes('\n')
		if err != nil {
			// An incomplete line waits for the rest
			p.buf.Write(line)
			return len(b), nil
		}
		fmt.Fprintf(p.w, "%s: %s", p.prefix, line)
	}
}

// Flush writes what is left of an unterminated last line
func (p *prefixWriter) Flush() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.buf.Len() > 0 {
		fmt.Fprintf(p.w, "%s: %s\n", p.prefix, p.buf.Bytes())
		p.buf.Reset()
	}
}
