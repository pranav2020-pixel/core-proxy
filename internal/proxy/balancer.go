package proxy

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	customHttp "core-proxy/internal/http"
)

type Backend struct {
	URL   *url.URL
	Alive bool
	mux   sync.RWMutex
}

func (b *Backend) SetAlive(alive bool) {
	b.mux.Lock()
	defer b.mux.Unlock()
	b.Alive = alive
}

func (b *Backend) IsAlive() bool {
	b.mux.RLock()
	defer b.mux.RUnlock()
	return b.Alive
}

type LoadBalancer struct {
	backends []*Backend
	current  uint64
}

func NewLoadBalancer(targetURLs []string) (*LoadBalancer, error) {
	var backends []*Backend
	for _, raw := range targetURLs {
		parsed, err := url.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid upstream URL '%s': %w", raw, err)
		}
		backends = append(backends, &Backend{
			URL:   parsed,
			Alive: true,
		})
	}

	lb := &LoadBalancer{backends: backends}
	go lb.startHealthChecks(5 * time.Second)
	return lb, nil
}

// NextBackend selects the next available backend using round-robin.
func (lb *LoadBalancer) NextBackend() *Backend {
	total := uint64(len(lb.backends))
	if total == 0 {
		return nil
	}

	for i := uint64(0); i < total; i++ {
		idx := atomic.AddUint64(&lb.current, 1) % total
		b := lb.backends[idx]
		if b.IsAlive() {
			return b
		}
	}

	return nil
}

// Forward relays req to a healthy backend and returns the backend's response.
func (lb *LoadBalancer) Forward(req *customHttp.Request) (*customHttp.Response, error) {
	target := lb.NextBackend()
	if target == nil {
		return customHttp.NewResponse(503, []byte("503 Service Unavailable: No healthy upstreams\n")), nil
	}

	host := target.URL.Host
	if !strings.Contains(host, ":") {
		host = host + ":80"
	}

	upstreamConn, err := net.DialTimeout("tcp", host, 3*time.Second)
	if err != nil {
		target.SetAlive(false)
		return customHttp.NewResponse(502, []byte(fmt.Sprintf("502 Bad Gateway: %v\n", err))), nil
	}
	defer upstreamConn.Close()

	clientIP, _, _ := net.SplitHostPort(req.RemoteAddr)
	if existing, ok := req.Headers["x-forwarded-for"]; ok {
		req.Headers["x-forwarded-for"] = existing + ", " + clientIP
	} else {
		req.Headers["x-forwarded-for"] = clientIP
	}
	req.Headers["host"] = target.URL.Host
	req.Headers["connection"] = "close"

	var b strings.Builder
	fmt.Fprintf(&b, "%s %s %s\r\n", req.Method, req.Path, req.Proto)
	for k, v := range req.Headers {
		fmt.Fprintf(&b, "%s: %s\r\n", k, v)
	}
	b.WriteString("\r\n")

	if _, err := io.WriteString(upstreamConn, b.String()); err != nil {
		return nil, fmt.Errorf("failed sending request to upstream: %w", err)
	}

	if len(req.Body) > 0 {
		if _, err := upstreamConn.Write(req.Body); err != nil {
			return nil, fmt.Errorf("failed sending request body to upstream: %w", err)
		}
	}

	upstreamReader := bufio.NewReader(upstreamConn)
	upstreamResp, err := customHttp.ParseRequest(upstreamReader, upstreamConn.RemoteAddr().String())
	if err != nil {
		rawResp, _ := io.ReadAll(upstreamReader)
		return customHttp.NewResponse(200, rawResp), nil
	}

	return customHttp.NewResponse(200, upstreamResp.Body), nil
}

func (lb *LoadBalancer) startHealthChecks(interval time.Duration) {
	ticker := time.NewTicker(interval)
	for range ticker.C {
		for _, b := range lb.backends {
			host := b.URL.Host
			if !strings.Contains(host, ":") {
				host = host + ":80"
			}

			conn, err := net.DialTimeout("tcp", host, 1*time.Second)
			if err != nil {
				if b.IsAlive() {
					log.Printf("backend offline: %s", b.URL.String())
					b.SetAlive(false)
				}
			} else {
				conn.Close()
				if !b.IsAlive() {
					log.Printf("backend recovered: %s", b.URL.String())
					b.SetAlive(true)
				}
			}
		}
	}
}
