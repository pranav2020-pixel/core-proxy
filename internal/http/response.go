package http

import (
	"fmt"
	"io"
	"strconv"
)

// Response represents an outbound HTTP/1.1 response.
type Response struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
}

// NewResponse initializes a response with default security & keep-alive headers.
func NewResponse(statusCode int, body []byte) *Response {
	resp := &Response{
		StatusCode: statusCode,
		Headers: map[string]string{
			"Server":       "core-proxy/1.0 (raw-socket)",
			"Connection":   "close",
			"Content-Type": "text/plain; charset=utf-8",
		},
		Body: body,
	}

	if len(body) > 0 {
		resp.Headers["Content-Length"] = strconv.Itoa(len(body))
	} else {
		resp.Headers["Content-Length"] = "0"
	}

	return resp
}

// StatusText returns standard HTTP reason phrases.
func StatusText(code int) string {
	switch code {
	case 200:
		return "OK"
	case 201:
		return "Created"
	case 400:
		return "Bad Request"
	case 403:
		return "Forbidden"
	case 404:
		return "Not Found"
	case 500:
		return "Internal Server Error"
	case 502:
		return "Bad Gateway"
	case 503:
		return "Service Unavailable"
	default:
		return "Unknown Status"
	}
}

// WriteTo writes the formatted HTTP/1.1 wire protocol directly into an output stream (e.g. net.Conn).
//
// Professor's Note:
// Format:
// HTTP/1.1 [StatusCode] [ReasonPhrase] CRLF
// [Header-Name]: [Header-Value] CRLF
// ...
// CRLF
// [Payload Bytes]
func (resp *Response) WriteTo(w io.Writer) error {
	statusLine := fmt.Sprintf("HTTP/1.1 %d %s\r\n", resp.StatusCode, StatusText(resp.StatusCode))
	if _, err := io.WriteString(w, statusLine); err != nil {
		return err
	}

	for k, v := range resp.Headers {
		headerLine := fmt.Sprintf("%s: %s\r\n", k, v)
		if _, err := io.WriteString(w, headerLine); err != nil {
			return err
		}
	}

	// Double CRLF marks the transition between HTTP headers and the binary payload
	if _, err := io.WriteString(w, "\r\n"); err != nil {
		return err
	}

	if len(resp.Body) > 0 {
		if _, err := w.Write(resp.Body); err != nil {
			return err
		}
	}

	return nil
}
