package server

import (
	"net"
	"runtime"
	"strings"
	"sync"
	"testing"
)

type fragConn struct {
	net.Conn
	mu  sync.Mutex
	buf strings.Builder
}

func (c *fragConn) Write(b []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, ch := range b {
		if err := c.buf.WriteByte(ch); err != nil {
			return i, err
		}
		runtime.Gosched()
	}
	return len(b), nil
}

func TestLockedConnKeepsLinesWhole(t *testing.T) {
	fake := &fragConn{}
	conn := &lockedConn{Conn: fake}

	const goroutines, lines, lineLen = 8, 50, 16

	var wg sync.WaitGroup
	for g := range goroutines {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			line := strings.Repeat(string(rune('a'+g)), lineLen)
			for range lines {
				if _, err := conn.Write([]byte(line + "\n")); err != nil {
					t.Errorf("write: %v", err)
					return
				}
			}
		}(g)
	}
	wg.Wait()

	got := strings.Split(strings.TrimSuffix(fake.buf.String(), "\n"), "\n")
	if len(got) != goroutines*lines {
		t.Fatalf("строк: %d, want %d", len(got), goroutines*lines)
	}
	for i, line := range got {
		if len(line) != lineLen || strings.Count(line, line[:1]) != lineLen {
			t.Fatalf("строка %d перемешана или порвана: %q", i, line)
		}
	}
}
