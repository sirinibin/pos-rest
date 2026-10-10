package main

import (
	"bufio"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// wsServerConn is the server side of a WebSocket for the mock machines.
type wsServerConn struct {
	c   net.Conn
	r   *bufio.Reader
	wmu sync.Mutex
}

func wsUpgrade(w http.ResponseWriter, r *http.Request) (*wsServerConn, error) {
	h, _ := w.(http.Hijacker)
	c, brw, err := h.Hijack()
	if err != nil {
		return nil, err
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	_, _ = io.WriteString(c, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: "+wsAccept(key)+"\r\n\r\n")
	return &wsServerConn{c: c, r: brw.Reader}, nil
}

func (s *wsServerConn) send(msg string) {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	p := []byte(msg)
	h := []byte{0x81}
	switch n := len(p); {
	case n < 126:
		h = append(h, byte(n))
	default:
		h = append(h, 126, byte(n>>8), byte(n))
	}
	_, _ = s.c.Write(append(h, p...))
}

func (s *wsServerConn) read() (string, error) {
	h := make([]byte, 2)
	if _, err := io.ReadFull(s.r, h); err != nil {
		return "", err
	}
	if h[0]&0x0f == 0x8 {
		return "", io.EOF
	}
	n := int(h[1] & 0x7f)
	if n == 126 {
		b := make([]byte, 2)
		_, _ = io.ReadFull(s.r, b)
		n = int(binary.BigEndian.Uint16(b))
	}
	mk := make([]byte, 4)
	if _, err := io.ReadFull(s.r, mk); err != nil {
		return "", err
	}
	p := make([]byte, n)
	if _, err := io.ReadFull(s.r, p); err != nil {
		return "", err
	}
	for i := range p {
		p[i] ^= mk[i%4]
	}
	return string(p), nil
}

// wsMock serves WebSocket connections; handle gets each text message.
func wsMock(t *testing.T, handle func(s *wsServerConn, msg string)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s, err := wsUpgrade(w, r)
		if err != nil {
			return
		}
		defer s.c.Close()
		for {
			m, err := s.read()
			if err != nil {
				return
			}
			handle(s, m)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func wsURL(srv *httptest.Server, path string) string {
	return "ws" + strings.TrimPrefix(srv.URL, "http") + path
}

func hostPort(srv *httptest.Server) string { return strings.TrimPrefix(srv.URL, "http://") }

func noProgress(string) {}

func waitFor(t *testing.T, d time.Duration, ok func() bool) {
	t.Helper()
	end := time.Now().Add(d)
	for time.Now().Before(end) {
		if ok() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("condition not met in time")
}
