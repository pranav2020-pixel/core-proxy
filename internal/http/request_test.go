package http

import (
	"bufio"
	"strings"
	"testing"
)

func TestParseValidGetRequest(t *testing.T) {
	rawHTTP := "GET /index.html HTTP/1.1\r\n" +
		"Host: localhost:8080\r\n" +
		"User-Agent: curl/7.68.0\r\n" +
		"Accept: */*\r\n" +
		"\r\n"

	reader := bufio.NewReader(strings.NewReader(rawHTTP))
	req, err := ParseRequest(reader, "127.0.0.1:54321")
	if err != nil {
		t.Fatalf("unexpected error parsing valid request: %v", err)
	}

	if req.Method != "GET" {
		t.Errorf("expected GET, got %s", req.Method)
	}
	if req.Path != "/index.html" {
		t.Errorf("expected /index.html, got %s", req.Path)
	}
	if req.Proto != "HTTP/1.1" {
		t.Errorf("expected HTTP/1.1, got %s", req.Proto)
	}
	if req.Headers["host"] != "localhost:8080" {
		t.Errorf("expected host header 'localhost:8080', got '%s'", req.Headers["host"])
	}
}

func TestParseRequestWithBody(t *testing.T) {
	bodyContent := `{"hello":"world"}`
	rawHTTP := "POST /api/echo HTTP/1.1\r\n" +
		"Host: localhost:8080\r\n" +
		"Content-Type: application/json\r\n" +
		"Content-Length: 17\r\n" +
		"\r\n" +
		bodyContent

	reader := bufio.NewReader(strings.NewReader(rawHTTP))
	req, err := ParseRequest(reader, "127.0.0.1:54321")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.Method != "POST" {
		t.Errorf("expected POST, got %s", req.Method)
	}
	if string(req.Body) != bodyContent {
		t.Errorf("expected body '%s', got '%s'", bodyContent, string(req.Body))
	}
}

func TestMalformedRequestLine(t *testing.T) {
	rawHTTP := "INVALID_REQUEST_LINE\r\n\r\n"
	reader := bufio.NewReader(strings.NewReader(rawHTTP))
	_, err := ParseRequest(reader, "127.0.0.1:54321")
	if err == nil {
		t.Fatalf("expected error on malformed request line, got nil")
	}
}
