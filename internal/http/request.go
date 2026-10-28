package http

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

var (
	ErrMalformedRequestLine = errors.New("malformed http request line")
	ErrMalformedHeader      = errors.New("malformed header entry")
	ErrBodyTooLarge         = errors.New("request payload exceeded maximum allowed limit")
)

const MaxHeaderBytes = 64 * 1024

type Request struct {
	Method     string
	Path       string
	Proto      string
	Headers    map[string]string
	Body       []byte
	RemoteAddr string
}

// ParseRequest reads and parses an HTTP/1.1 wire request from r.
func ParseRequest(r *bufio.Reader, remoteAddr string) (*Request, error) {
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

	totalHeaderSize := 0
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}

		totalHeaderSize += len(line)
		if totalHeaderSize > MaxHeaderBytes {
			return nil, errors.New("header size exceeds limit")
		}

		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed == "" {
			break
		}

		colonIdx := strings.Index(trimmed, ":")
		if colonIdx == -1 {
			return nil, fmt.Errorf("%w: '%s'", ErrMalformedHeader, trimmed)
		}

		key := strings.ToLower(strings.TrimSpace(trimmed[:colonIdx]))
		val := strings.TrimSpace(trimmed[colonIdx+1:])
		req.Headers[key] = val
	}

	if clStr, ok := req.Headers["content-length"]; ok {
		contentLength, err := strconv.Atoi(clStr)
		if err == nil && contentLength > 0 {
			if contentLength > 10*1024*1024 {
				return nil, ErrBodyTooLarge
			}
			bodyBuf := make([]byte, contentLength)
			if _, err = io.ReadFull(r, bodyBuf); err != nil {
				return nil, fmt.Errorf("failed reading body: %w", err)
			}
			req.Body = bodyBuf
		}
	}

	return req, nil
}
