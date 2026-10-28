# core-proxy ⚡

A high-performance, multi-threaded **HTTP/1.1 Server & Reverse Proxy** implemented from first principles in **Go** using raw BSD socket primitives (`net.Listen`). Built with **zero external web frameworks** (no Gin, no Chi, no standard library `net/http` server).

---

## Architecture Overview

```
                      ┌─────────────────────────────────────────┐
                      │          Incoming TCP Client            │
                      └────────────────────┬────────────────────┘
                                           │  TCP 3-Way Handshake
                                           ▼
                      ┌─────────────────────────────────────────┐
                      │    Raw Socket Listener (net.Listen)     │
                      └────────────────────┬────────────────────┘
                                           │  go handleConnection()
                                           ▼
                      ┌─────────────────────────────────────────┐
                      │      Streaming HTTP/1.1 Parser          │
                      │  - Request-Line (Method, Path, Proto)   │
                      │  - RFC 7230 CRLF Delimited Headers      │
                      │  - Content-Length Bound Payload         │
                      └────────────────────┬────────────────────┘
                                           │
                         ┌─────────────────┴─────────────────┐
                         ▼                                   ▼
        ┌─────────────────────────────────┐ ┌─────────────────────────────────┐
        │        Static File Server       │ │     Round-Robin Reverse Proxy   │
        │ - Directory Traversal Guard     │ │ - Upstream Health Checker       │
        │ - Extension MIME Type Resolver  │ │ - X-Forwarded-For Header Injection│
        └─────────────────────────────────┘ └─────────────────────────────────┘
```

---

## Key Features

- **Raw TCP Wire Parsing:** Conforms to RFC 7230, reading directly from buffered socket descriptors (`bufio.Reader`).
- **Zero Framework Bloat:** Hand-crafted request tokenization and binary response serialization.
- **Round-Robin Load Balancing:** Thread-safe, lock-free backend selection using atomic pointer arithmetic (`sync/atomic`).
- **Active Health Probing:** Background goroutine periodically checks upstream availability and removes failing servers from the active pool.
- **Defensive Systems Programming:**
  - Connection deadlines (`SetDeadline`) to mitigate Slowloris resource exhaustion.
  - Path canonicalization (`filepath.Clean`) preventing directory traversal attacks (`/../../etc/passwd`).
  - Max header byte boundaries preventing heap allocation exhaustion.

---

## Quickstart

### 1. Build and Run
```bash
# Clone the repository
git clone https://github.com/your-username/core-proxy.git
cd core-proxy

# Run the server
go run main.go -port 8080 -static ./public -upstreams "http://localhost:9001,http://localhost:9002"
```

### 2. Verify with curl
```bash
# Test static file serving
curl -i http://localhost:8080/

# Test health check endpoint
curl -i http://localhost:8080/api/health

# Test echo endpoint
curl -X POST -d '{"msg":"systems-programming"}' http://localhost:8080/api/echo
```

### 3. Run Automated Tests
```bash
# Run unit tests with Go's race condition detector
go test -v -race ./...
```

---

## Core CS Concepts Mastered

1. **Operating Systems:** Socket system calls (`socket`, `bind`, `listen`, `accept`), file descriptors, and non-blocking worker thread pools.
2. **Computer Networks:** The HTTP/1.1 wire protocol, Keep-Alive connection semantics, header tokenization, and proxy forwarding.
3. **Concurrency:** Atomic operations (`atomic.AddUint64`), Reader-Writer mutexes (`sync.RWMutex`), and goroutine lifecycles.
