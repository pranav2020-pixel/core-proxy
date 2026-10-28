package http

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Common HTTP errors encountered when parsing raw wire streams.
var (
	ErrMalformedRequestLine = errors.New("malformed http request line")
	ErrMalformedHeader      = errors.New("malformed header entry")
	ErrBodyTooLarge         = errors.New("request payload exceeded maximum allowed limit")
)

// MaxHeaderBytes defines a defensive boundary against Slowloris and memory exhaustion attacks.
const MaxHeaderBytes = 64 * 1024 // 64 KB

// Request represents a parsed HTTP/1.1 request conforming to RFC 7230.
type Request struct {
	Method     string            // GET, POST, PUT, DELETE, etc.
	Path       string            // Requested URI path (e.g., /index.html)
	Proto      string            // HTTP/1.1
	Headers    map[string]string // Key-value headers stored in lowercase for fast, normalized lookup
	Body       []byte            // Payload bytes (if Content-Length is specified)
	RemoteAddr string            // The client's IP:Port as reported by the TCP socket
}

// ParseRequest streams bytes from a buffered TCP socket and constructs a structured Request.
//
// Professor's Note:
// In the HTTP/1.1 wire specification, headers are ASCII strings delimited by CRLF (\r\n).
// The first line is the "Request-Line" containing: [Method] [SP] [Request-URI] [SP] [HTTP-Version] CRLF.
// Following the Request-Line are zero or more "Header-Field" lines.
// An empty CRLF line marks the end of headers and the beginning of the body.
func ParseRequest(r *bufio.Reader, remoteAddr string) (*Request, error) {
	// 1. Read the Request-Line
	requestLine, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}

	requestLine = strings.TrimRight(requestLine, "\r\n")
	parts := strings.Split(requestLine, " ")
	if len(parts) != 3 {
		return nil, fmt.Errorf("%w: '%s'", ErrMalformedRequestLine, requestLine)
	}

	req := &Request{
		Method:     strings.ToUpper(parts[0]),
		Path:       parts[1],
		Proto:      parts[2],
		Headers:    make(map[string]string),
		RemoteAddr: remoteAddr,
	}

	// 2. Read Headers iteratively until encountering an empty line ("\r\n" or "")
	totalHeaderSize := 0
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}

		totalHeaderSize += len(line)
		if totalHeaderSize > MaxHeaderBytes {
			return nil, errors.New("header size exceeds security threshold")
		}

		// CRLF on an empty line means we reached the end of headers
		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed == "" {
			break
		}

		// Header format: "Key: Value"
		colonIdx := strings.Index(trimmed, ":")
		if colonIdx == -1 {
			return nil, fmt.Errorf("%w: '%s'", ErrMalformedHeader, trimmed)
		}

		key := strings.ToLower(strings.TrimSpace(trimmed[:colonIdx]))
		val := strings.TrimSpace(trimmed[colonIdx+1:])
		req.Headers[key] = val
	}

	// 3. Read Body (if Content-Length header is present)
	if clStr, ok := req.Headers["content-length"]; ok {
		contentLength, err := strconv.Atoi(clStr)
		if err == nil && contentLength > 0 {
			if contentLength > 10*1024*1024 { // 10 MB sanity limit
				return nil, ErrBodyTooLarge
			}
			bodyBuf := make([]byte, contentLength)
			_, err = io.ReadFull(r, bodyBuf)
			if err != nil {
				return nil, fmt.Errorf("failed reading request body: %w", err)
			}
			req.Body = bodyBuf
		}
	}

	return req, nil
}
