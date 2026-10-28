package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	customHttp "core-proxy/internal/http"
	"core-proxy/internal/proxy"
)

func main() {
	port := flag.Int("port", 8080, "Port on which the TCP listener binds")
	staticDir := flag.String("static", "./public", "Directory containing static assets")
	upstreams := flag.String("upstreams", "http://example.com", "Comma-separated upstream URLs for reverse proxy")
	flag.Parse()

	// 1. Initialize Load Balancer for proxying
	upstreamList := strings.Split(*upstreams, ",")
	balancer, err := proxy.NewLoadBalancer(upstreamList)
	if err != nil {
		log.Fatalf("Failed to initialize load balancer: %v", err)
	}

	// 2. Open Raw BSD TCP Socket Listener
	//
	// Professor's Note:
	// net.Listen invokes the 'socket(2)', 'bind(2)', and 'listen(2)' system calls in the OS kernel.
	// It queues incoming TCP connections in the OS SYN backlog until accept() is called.
	addr := fmt.Sprintf(":%d", *port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("Failed to bind socket on %s: %v", addr, err)
	}
	defer listener.Close()

	fmt.Printf("\n🚀 [core-proxy] Server listening on http://localhost:%d\n", *port)
	fmt.Printf("📁 [core-proxy] Serving static files from: %s\n", *staticDir)
	fmt.Printf("🔄 [core-proxy] Reverse proxy upstreams: %v\n\n", upstreamList)

	// 3. Main Connection Accept Loop
	for {
		// listener.Accept() blocks until a client finishes the TCP 3-way handshake
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Error accepting TCP connection: %v", err)
			continue
		}

		// Set connection deadline to mitigate slowloris attacks
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

		// Dispatch connection to an isolated goroutine
		go handleConnection(conn, *staticDir, balancer)
	}
}

func handleConnection(conn net.Conn, staticDir string, balancer *proxy.LoadBalancer) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	req, err := customHttp.ParseRequest(reader, conn.RemoteAddr().String())
	if err != nil {
		resp := customHttp.NewResponse(400, []byte(fmt.Sprintf("400 Bad Request: %v\n", err)))
		_ = resp.WriteTo(conn)
		return
	}

	start := time.Now()
	var resp *customHttp.Response

	// Route Dispatcher
	switch {
	case strings.HasPrefix(req.Path, "/proxy"):
		// Strip /proxy prefix and route to upstream pool
		req.Path = strings.TrimPrefix(req.Path, "/proxy")
		if req.Path == "" {
			req.Path = "/"
		}
		var forwardErr error
		resp, forwardErr = balancer.Forward(req)
		if forwardErr != nil {
			resp = customHttp.NewResponse(502, []byte(fmt.Sprintf("502 Bad Gateway: %v\n", forwardErr)))
		}

	case req.Path == "/api/health":
		jsonPayload := fmt.Sprintf(`{"status":"UP","timestamp":"%s","remote_addr":"%s"}`+"\n", time.Now().UTC().Format(time.RFC3339), req.RemoteAddr)
		resp = customHttp.NewResponse(200, []byte(jsonPayload))
		resp.Headers["Content-Type"] = "application/json"

	case req.Path == "/api/echo":
		resp = customHttp.NewResponse(200, req.Body)
		resp.Headers["Content-Type"] = "application/octet-stream"

	default:
		// Attempt static file resolution
		filePath := filepath.Join(staticDir, req.Path)
		if req.Path == "/" {
			filePath = filepath.Join(staticDir, "index.html")
		}

		// Security check: Prevent directory traversal (e.g. /../../etc/passwd)
		cleanPath := filepath.Clean(filePath)
		if !strings.HasPrefix(cleanPath, filepath.Clean(staticDir)) {
			resp = customHttp.NewResponse(403, []byte("403 Forbidden: Directory traversal is blocked\n"))
		} else if fileBytes, err := os.ReadFile(cleanPath); err == nil {
			resp = customHttp.NewResponse(200, fileBytes)
			resp.Headers["Content-Type"] = resolveMimeType(cleanPath)
		} else {
			resp = customHttp.NewResponse(404, []byte(fmt.Sprintf("404 Not Found: %s does not exist\n", req.Path)))
		}
	}

	_ = resp.WriteTo(conn)
	duration := time.Since(start)
	log.Printf("[%s] %s %s -> %d (%v)", req.RemoteAddr, req.Method, req.Path, resp.StatusCode, duration)
}

func resolveMimeType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js":
		return "application/javascript; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".svg":
		return "image/svg+xml"
	default:
		return "text/plain; charset=utf-8"
	}
}
