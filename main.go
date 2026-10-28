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
	port := flag.Int("port", 8080, "listen port")
	staticDir := flag.String("static", "./public", "static assets directory")
	upstreams := flag.String("upstreams", "", "comma-separated upstream backend URLs")
	flag.Parse()

	var balancer *proxy.LoadBalancer
	if *upstreams != "" {
		var err error
		balancer, err = proxy.NewLoadBalancer(strings.Split(*upstreams, ","))
		if err != nil {
			log.Fatalf("load balancer init error: %v", err)
		}
	}

	addr := fmt.Sprintf(":%d", *port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("failed to bind %s: %v", addr, err)
	}
	defer listener.Close()

	log.Printf("core-proxy listening on %s", addr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("accept error: %v", err)
			continue
		}

		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
		go handleConnection(conn, *staticDir, balancer)
	}
}

func handleConnection(conn net.Conn, staticDir string, balancer *proxy.LoadBalancer) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	req, err := customHttp.ParseRequest(reader, conn.RemoteAddr().String())
	if err != nil {
		resp := customHttp.NewResponse(400, []byte("400 Bad Request\n"))
		_ = resp.WriteTo(conn)
		return
	}

	start := time.Now()
	var resp *customHttp.Response

	switch {
	case strings.HasPrefix(req.Path, "/proxy"):
		if balancer == nil {
			resp = customHttp.NewResponse(503, []byte("no backends configured\n"))
			break
		}
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
		payload := fmt.Sprintf(`{"status":"UP","timestamp":"%s"}`+"\n", time.Now().UTC().Format(time.RFC3339))
		resp = customHttp.NewResponse(200, []byte(payload))
		resp.Headers["Content-Type"] = "application/json"

	case req.Path == "/api/echo":
		resp = customHttp.NewResponse(200, req.Body)
		resp.Headers["Content-Type"] = "application/octet-stream"

	default:
		filePath := filepath.Join(staticDir, req.Path)
		if req.Path == "/" {
			filePath = filepath.Join(staticDir, "index.html")
		}

		cleanPath := filepath.Clean(filePath)
		if !strings.HasPrefix(cleanPath, filepath.Clean(staticDir)) {
			resp = customHttp.NewResponse(403, []byte("403 Forbidden\n"))
		} else if fileBytes, err := os.ReadFile(cleanPath); err == nil {
			resp = customHttp.NewResponse(200, fileBytes)
			resp.Headers["Content-Type"] = resolveMimeType(cleanPath)
		} else {
			resp = customHttp.NewResponse(404, []byte("404 Not Found\n"))
		}
	}

	_ = resp.WriteTo(conn)
	log.Printf("%s %s %s %d %v", req.RemoteAddr, req.Method, req.Path, resp.StatusCode, time.Since(start))
}

func resolveMimeType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
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
