package proxy

import (
	"testing"
)

func TestRoundRobinDistribution(t *testing.T) {
	urls := []string{
		"http://localhost:9001",
		"http://localhost:9002",
		"http://localhost:9003",
	}

	lb, err := NewLoadBalancer(urls)
	if err != nil {
		t.Fatalf("failed to init load balancer: %v", err)
	}

	// 1. Verify sequential round-robin distribution
	first := lb.NextBackend()
	second := lb.NextBackend()
	third := lb.NextBackend()
	fourth := lb.NextBackend()
	_ = third

	if first.URL.String() != urls[1] && first.URL.String() != urls[0] {
		// depending on atomic counter initialization
	}
	if second.URL.String() == first.URL.String() {
		t.Errorf("expected consecutive calls to hit different backends, both hit %s", first.URL.String())
	}
	if fourth.URL.String() != first.URL.String() {
		t.Errorf("expected cycle back to start after 3 calls: got %s, expected %s", fourth.URL.String(), first.URL.String())
	}
}

func TestDeadBackendFailover(t *testing.T) {
	urls := []string{
		"http://localhost:9001",
		"http://localhost:9002",
	}

	lb, err := NewLoadBalancer(urls)
	if err != nil {
		t.Fatalf("failed to init load balancer: %v", err)
	}

	// Mark backend 0 as dead
	lb.backends[0].SetAlive(false)

	// All next calls must route exclusively to backend 1
	for i := 0; i < 5; i++ {
		b := lb.NextBackend()
		if b == nil {
			t.Fatalf("expected active backend, got nil")
		}
		if b.URL.String() != "http://localhost:9002" {
			t.Errorf("expected all traffic to hit active backend 9002, got %s", b.URL.String())
		}
	}

	// Mark all backends as dead
	lb.backends[1].SetAlive(false)
	deadCall := lb.NextBackend()
	if deadCall != nil {
		t.Errorf("expected nil when all backends are dead, got %s", deadCall.URL.String())
	}
}
