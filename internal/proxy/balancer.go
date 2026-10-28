package proxy

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	customHttp "core-proxy/internal/http"
)

// Backend represents a target upstream service.
type Backend struct {
	URL          *url.URL
	Alive        bool
	mux          sync.RWMutex
	Failures     int
	SuccessCount int
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

// LoadBalancer coordinates request routing across healthy upstream services.
type LoadBalancer struct {
	backends []*Backend
	current  uint64
}

// NewLoadBalancer initializes backends from raw URL strings.
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

// NextBackend selects the next available upstream using lock-free atomic Round-Robin.
func (lb *LoadBalancer) NextBackend() *Backend {
	total := uint64(len(lb.backends))
	if total == 0 {
		return nil
	}

	// Try up to `total` times to find an alive backend
	for i := uint64(0); i < total; i++ {
		idx := atomic.AddUint64(&lb.current, 1) % total
		b := lb.backends[idx]
		if b.IsAlive() {
			return b
		}
	}

	return nil // All upstreams are currently down
}

// Forward transmits the client's request to the selected upstream backend,
// mutating headers to conform with standard reverse proxy conventions (X-Forwarded-For).
func (lb *LoadBalancer) Forward(req *customHttp.Request) (*customHttp.Response, error) {
	target := lb.NextBackend()
	if target == nil {
		return customHttp.NewResponse(503, []byte("503 Service Unavailable: All upstream servers are offline\n")), nil
	}

	// Dial the upstream host over raw TCP
	host := target.URL.Host
	if !strings.Contains(host, ":") {
		host = host + ":80"
	}

	upstreamConn, err := net.DialTimeout("tcp", host, 3*time.Second)
	if err != nil {
		target.SetAlive(false)
		return customHttp.NewResponse(502, []byte(fmt.Sprintf("502 Bad Gateway: Failed to connect to %s\n", target.URL.String()))), nil
	}
	defer upstreamConn.Close()

	// 1. Construct modified headers for the upstream server
	// Append client IP to X-Forwarded-For
	clientIP, _, _ := net.SplitHostPort(req.RemoteAddr)
	if existing, ok := req.Headers["x-forwarded-for"]; ok {
		req.Headers["x-forwarded-for"] = existing + ", " + clientIP
	} else {
		req.Headers["x-forwarded-for"] = clientIP
	}
	req.Headers["host"] = target.URL.Host
	req.Headers["connection"] = "close"

	// 2. Transmit the request to the upstream server
	requestBuffer := fmt.Sprintf("%s %s %s\r\n", req.Method, req.Path, req.Proto)
	for k, v := range req.Headers {
		requestBuffer += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	requestBuffer += "\r\n"

	if _, err := io.WriteString(upstreamConn, requestBuffer); err != nil {
		return nil, fmt.Errorf("failed sending request to upstream: %w", err)
	}

	if len(req.Body) > 0 {
		if _, err := upstreamConn.Write(req.Body); err != nil {
			return nil, fmt.Errorf("failed sending request body to upstream: %w", err)
		}
	}

	// 3. Read the upstream response using custom parser
	upstreamReader := bufio.NewReader(upstreamConn)
	upstreamResp, err := customHttp.ParseRequest(upstreamReader, upstreamConn.RemoteAddr().String())
	if err != nil {
		// If upstream returns a direct standard response, read until EOF
		rawResp, _ := io.ReadAll(upstreamReader)
		return customHttp.NewResponse(200, rawResp), nil
	}

	return customHttp.NewResponse(200, upstreamResp.Body), nil
}

// startHealthChecks periodically polls upstreams to update their availability state.
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
					fmt.Printf("[HEALTH CHECK] ❌ Backend down: %s\n", b.URL.String())
					b.SetAlive(false)
				}
			} else {
				conn.Close()
				if !b.IsAlive() {
					fmt.Printf("[HEALTH CHECK] ✅ Backend recovered: %s\n", b.URL.String())
					b.SetAlive(true)
				}
			}
		}
	}
}
