package gateway

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	socksVersion = 5
	cmdConnect   = 1
	atypIPv4     = 1
	atypDomain   = 3
	atypIPv6     = 4
)

type Server struct {
	policy *Policy
	ln     net.Listener

	closeOnce sync.Once
	closed    chan struct{}
	mu        sync.Mutex
	conns     map[net.Conn]struct{}
	wg        sync.WaitGroup
}

func NewServer(p *Policy, listenAddr string) (*Server, error) {
	if p == nil {
		return nil, errors.New("gateway: policy is required")
	}
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, err
	}
	return &Server{policy: p, ln: ln, closed: make(chan struct{}), conns: make(map[net.Conn]struct{})}, nil
}

func (s *Server) Addr() net.Addr { return s.ln.Addr() }

func (s *Server) Serve() {
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		select {
		case <-s.closed:
			s.mu.Unlock()
			_ = c.Close()
			return
		default:
		}
		s.conns[c] = struct{}{}
		s.wg.Add(1)
		s.mu.Unlock()
		go func() {
			defer s.wg.Done()
			defer func() { s.mu.Lock(); delete(s.conns, c); s.mu.Unlock() }()
			s.handle(c)
		}()
	}
}

func (s *Server) Close() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		close(s.closed)
		_ = s.ln.Close()
		for c := range s.conns {
			_ = c.Close()
		}
		s.mu.Unlock()
	})
	// Abort target sockets too, including transfers blocked on the target.
	s.policy.AbortConnections()
	s.wg.Wait()
	return s.policy.Close()
}

// DiscardListener releases a listener that has not entered Serve yet.
func (s *Server) DiscardListener() error { return s.ln.Close() }

func (s *Server) handle(c net.Conn) {
	var once sync.Once
	var targetMu sync.Mutex
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var target net.Conn
	abort := func() {
		once.Do(func() {
			cancel()
			targetMu.Lock()
			defer targetMu.Unlock()
			_ = c.Close()
			if target != nil {
				_ = target.Close()
			}
		})
	}
	defer abort()

	_ = c.SetDeadline(time.Now().Add(15 * time.Second))
	br := bufio.NewReader(c)
	if !s.greet(c, br) {
		return
	}
	host, port, cmd, ok := readRequest(br)
	if !ok {
		_ = writeReply(c, 7)
		return
	}
	if cmd != cmdConnect {
		_ = writeReply(c, 7)
		return
	}
	host = normalizeHost(host)
	if host == "" || port == 0 {
		_ = writeReply(c, 8)
		return
	}
	id, tracked := s.policy.Track(host, abort)
	if !tracked {
		_ = writeReply(c, 2)
		return
	}

	defer s.policy.Untrack(id)
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		_ = writeReply(c, 5)
		return
	}
	targetMu.Lock()
	if ctx.Err() != nil {
		targetMu.Unlock()
		_ = conn.Close()
		return
	}
	target = conn
	targetMu.Unlock()

	_ = c.SetDeadline(time.Time{})
	if err := writeReply(c, 0); err != nil {
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		copyDirection(target, br, func(n int64) { s.policy.AccountConn(id, n, 0) })
	}()
	go func() {
		defer wg.Done()
		copyDirection(c, target, func(n int64) { s.policy.AccountConn(id, 0, n) })
	}()
	wg.Wait()
}

func (s *Server) greet(c net.Conn, br *bufio.Reader) bool {
	hdr := make([]byte, 2)
	if _, err := io.ReadFull(br, hdr); err != nil {
		return false
	}
	if hdr[0] != socksVersion {
		return false
	}
	methods := make([]byte, int(hdr[1]))
	if _, err := io.ReadFull(br, methods); err != nil {
		return false
	}
	for _, m := range methods {
		if m == 0 {
			if _, err := c.Write([]byte{5, 0}); err != nil {
				return false
			}
			return true
		}
	}
	_, _ = c.Write([]byte{5, 0xff})
	return false
}

func readRequest(br *bufio.Reader) (host string, port int, cmd byte, ok bool) {
	hdr := make([]byte, 4)
	if _, err := io.ReadFull(br, hdr); err != nil {
		return "", 0, 0, false
	}
	if hdr[0] != socksVersion {
		return "", 0, 0, false
	}
	cmd = hdr[1]
	switch hdr[3] {
	case atypIPv4:
		raw := make([]byte, 4)
		if _, err := io.ReadFull(br, raw); err != nil {
			return "", 0, 0, false
		}
		host = net.IP(raw).String()
	case atypIPv6:
		raw := make([]byte, 16)
		if _, err := io.ReadFull(br, raw); err != nil {
			return "", 0, 0, false
		}
		host = net.IP(raw).String()
	case atypDomain:
		n := make([]byte, 1)
		if _, err := io.ReadFull(br, n); err != nil || n[0] == 0 {
			return "", 0, 0, false
		}
		raw := make([]byte, int(n[0]))
		if _, err := io.ReadFull(br, raw); err != nil {
			return "", 0, 0, false
		}
		host = string(raw)
	default:
		return "", 0, 0, false
	}
	p := make([]byte, 2)
	if _, err := io.ReadFull(br, p); err != nil {
		return "", 0, 0, false
	}
	return host, int(binary.BigEndian.Uint16(p)), cmd, true
}

func writeReply(w io.Writer, code byte) error {
	_, err := w.Write([]byte{5, code, 0, 1, 0, 0, 0, 0, 0, 0})
	return err
}

func copyDirection(dst io.Writer, src io.Reader, account func(int64)) {
	buf := make([]byte, 32*1024)
	for {
		n, rerr := src.Read(buf)
		if n > 0 {
			written, werr := dst.Write(buf[:n])
			if written > 0 {
				account(int64(written))
			}
			if werr != nil {
				closeWrite(dst)
				return
			}
		}
		if rerr != nil {
			closeWrite(dst)
			return
		}
	}
}

func closeWrite(v any) {
	type closeWriter interface{ CloseWrite() error }
	if cw, ok := v.(closeWriter); ok {
		_ = cw.CloseWrite()
	}
}

func normalizeHost(host string) string {
	host = strings.TrimSpace(host)
	host = strings.TrimSuffix(host, ".")
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return strings.ToLower(host)
}
