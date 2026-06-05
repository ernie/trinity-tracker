package collector

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/ernie/trinity-tracker/internal/console"
)

const (
	conTapDialTimeout  = 2 * time.Second
	conTapHelloTimeout = 3 * time.Second
	conTapBackoffMax   = 15 * time.Second
	// conTapUnavailablePoll paces rediscovery when the server doesn't
	// offer a tap (old engine, sv_conTap 0).
	conTapUnavailablePoll = 30 * time.Second
)

// conTapRunner keeps one server's console tap attached: discover the
// kernel-assigned port from serverinfo, dial, validate the hello, feed
// the ring. Reconnects always rediscover — never redial a stale port.
type conTapRunner struct {
	key      string
	gameAddr string // 127.0.0.1:<game port> — the stable identity
	gamePort string
	ring     *console.Ring
	done     chan struct{}

	// statusVars fetches serverinfo; tests stub it.
	statusVars func() (map[string]string, error)
	// backoffBase shrinks in tests.
	backoffBase time.Duration
}

func newConTapRunner(key, gamePort string, ring *console.Ring, q3 *Q3Client, done chan struct{}) *conTapRunner {
	addr := net.JoinHostPort("127.0.0.1", gamePort)
	return &conTapRunner{
		key:      key,
		gameAddr: addr,
		gamePort: gamePort,
		ring:     ring,
		done:     done,
		statusVars: func() (map[string]string, error) {
			st, err := q3.QueryStatus(addr)
			if err != nil {
				return nil, err
			}
			return st.ServerVars, nil
		},
		backoffBase: time.Second,
	}
}

// run is the persistent loop. wasUp gates the reconnect marker so a
// server that never offered a tap doesn't accumulate noise.
func (t *conTapRunner) run() {
	backoff := t.backoffBase
	wasUp := false
	for {
		select {
		case <-t.done:
			return
		default:
		}

		port := t.discover()
		if port == 0 {
			t.ring.SetTapUp(false)
			if !t.sleep(conTapUnavailablePoll) {
				return
			}
			continue
		}

		conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), conTapDialTimeout)
		if err != nil {
			if !t.sleep(backoff) {
				return
			}
			backoff = minDuration(backoff*2, conTapBackoffMax)
			continue
		}

		if err := t.consume(conn, wasUp); err != nil {
			log.Printf("collector: console tap %s: %v", t.key, err)
		}
		t.ring.SetTapUp(false)
		select {
		case <-t.done:
			return
		default:
		}
		t.ring.Append(fmt.Sprintf("--- console connection lost (%s) ---", t.key))
		wasUp = true
		backoff = t.backoffBase
		if !t.sleep(backoff) {
			return
		}
	}
}

// discover reads sv_conport from serverinfo; 0 = no tap offered.
func (t *conTapRunner) discover() int {
	vars, err := t.statusVars()
	if err != nil {
		return 0
	}
	port, err := strconv.Atoi(vars["sv_conport"])
	if err != nil || port <= 0 || port > 65535 {
		return 0
	}
	return port
}

// consume validates the CON1 hello (identity: the game port must match
// ours — guards against the kernel reusing a stale ephemeral port) and
// feeds lines into the ring until the connection drops.
func (t *conTapRunner) consume(conn net.Conn, wasUp bool) error {
	defer conn.Close()

	// Unblock the read at shutdown.
	connDone := make(chan struct{})
	defer close(connDone)
	go func() {
		select {
		case <-t.done:
			conn.Close()
		case <-connDone:
		}
	}()

	conn.SetReadDeadline(time.Now().Add(conTapHelloTimeout))
	r := bufio.NewReader(conn)
	hello, err := r.ReadString('\n')
	if err != nil {
		return fmt.Errorf("hello read: %w", err)
	}
	fields := strings.Fields(hello)
	if len(fields) < 2 || fields[0] != "CON1" || fields[1] != t.gamePort {
		return fmt.Errorf("bad hello %q (want CON1 %s ...)", strings.TrimSpace(hello), t.gamePort)
	}
	conn.SetReadDeadline(time.Time{})

	t.ring.SetTapUp(true)
	if wasUp {
		t.ring.Append(fmt.Sprintf("--- console reconnected (%s) ---", t.key))
	}

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 4096), 64*1024)
	for sc.Scan() {
		t.ring.Append(strings.TrimRight(sc.Text(), "\r"))
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("read: %w", err)
	}
	return nil // EOF: server shut down
}

// sleep waits d or until shutdown; false = shutting down.
func (t *conTapRunner) sleep(d time.Duration) bool {
	select {
	case <-t.done:
		return false
	case <-time.After(d):
		return true
	}
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
