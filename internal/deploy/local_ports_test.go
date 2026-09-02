// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"testing"
)

type localPortTestListener struct {
	closed *int
}

func (*localPortTestListener) Accept() (net.Conn, error) { return nil, errors.New("unused") }
func (listener *localPortTestListener) Close() error {
	*listener.closed++
	return nil
}
func (*localPortTestListener) Addr() net.Addr { return nil }

func testLocalPortAllocator(
	t *testing.T,
	minimumPort, maximumPort int,
	available func(string, int) bool,
) localPortAllocator {
	t.Helper()
	closed := 0

	return localPortAllocator{
		lookupIP: func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
		},
		listen: func(_ string, address string) (net.Listener, error) {
			host, rawPort, err := net.SplitHostPort(address)
			if err != nil {
				t.Fatalf("invalid test listen address %q: %v", address, err)
			}
			port, err := strconv.Atoi(rawPort)
			if err != nil {
				t.Fatalf("invalid test listen port %q: %v", rawPort, err)
			}
			if !available(host, port) {
				return nil, errors.New("address unavailable")
			}

			return &localPortTestListener{closed: &closed}, nil
		},
		minimumPort: minimumPort,
		maximumPort: maximumPort,
	}
}

func TestLocalPortAllocator_SelectsDefaultThenAdvancesAndWraps(t *testing.T) {
	t.Parallel()

	service := []localService{{name: "db", guestPort: 4, defaultHostPort: 4}}
	tests := []struct {
		name      string
		available func(int) bool
		want      string
	}{
		{name: "default", available: func(port int) bool { return port == 4 }, want: "db:4"},
		{name: "advance", available: func(port int) bool { return port == 5 }, want: "db:5"},
		{name: "wrap", available: func(port int) bool { return port == 2 }, want: "db:2"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			allocator := testLocalPortAllocator(
				t, 2, 5, func(_ string, port int) bool { return test.available(port) },
			)

			actual, err := allocator.resolve(context.Background(), "", service)
			if err != nil {
				t.Fatalf("expected allocation to succeed, got %v", err)
			}
			if actual != test.want {
				t.Fatalf("expected %q, got %q", test.want, actual)
			}
		})
	}
}

func TestLocalPortAllocator_ReportsExhaustedService(t *testing.T) {
	t.Parallel()

	allocator := testLocalPortAllocator(t, 2, 5, func(string, int) bool { return false })
	_, err := allocator.resolve(
		context.Background(),
		"",
		[]localService{{name: "db", guestPort: 4, defaultHostPort: 4}},
	)
	if err == nil || !strings.Contains(err.Error(), `service "db"`) {
		t.Fatalf("expected exhausted database service error, got %v", err)
	}
}

func TestLocalPortAllocator_RequiresCandidateOnEveryLoopback(t *testing.T) {
	t.Parallel()

	closed := 0
	allocator := localPortAllocator{
		lookupIP: func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{
				{IP: net.ParseIP("127.0.0.1")},
				{IP: net.ParseIP("::1")},
				{IP: net.ParseIP("127.0.0.1")},
				{IP: net.ParseIP("192.0.2.1")},
			}, nil
		},
		listen: func(_ string, address string) (net.Listener, error) {
			host, rawPort, err := net.SplitHostPort(address)
			if err != nil {
				t.Fatalf("invalid test address %q: %v", address, err)
			}
			port, err := strconv.Atoi(rawPort)
			if err != nil {
				t.Fatalf("invalid test port %q: %v", rawPort, err)
			}
			if port == 4 && host == "::1" {
				return nil, errors.New("IPv6 unavailable")
			}

			return &localPortTestListener{closed: &closed}, nil
		},
		minimumPort: 2,
		maximumPort: 5,
	}

	actual, err := allocator.resolve(
		context.Background(),
		"",
		[]localService{{name: "db", guestPort: 4, defaultHostPort: 4}},
	)
	if err != nil {
		t.Fatalf("expected allocation to succeed, got %v", err)
	}
	if actual != "db:5" {
		t.Fatalf("expected db:5 after IPv6 collision, got %q", actual)
	}
	if closed != 3 {
		t.Fatalf("expected failed and selected candidate listeners to close, got %d", closed)
	}
}

func TestLocalPortAllocator_AssignsDistinctPortsToServices(t *testing.T) {
	t.Parallel()

	allocator := testLocalPortAllocator(t, 2, 5, func(string, int) bool { return true })
	actual, err := allocator.resolve(
		context.Background(),
		"",
		[]localService{
			{name: "db", guestPort: 4, defaultHostPort: 4},
			{name: "ui", guestPort: 4, defaultHostPort: 4},
		},
	)
	if err != nil {
		t.Fatalf("expected allocation to succeed, got %v", err)
	}
	if actual != "db:4,ui:5" {
		t.Fatalf("expected distinct service ports, got %q", actual)
	}
}

func TestLocalPortAllocator_PreservesExplicitMappings(t *testing.T) {
	t.Parallel()

	listenCalls := 0
	allocator := testLocalPortAllocator(t, 2, 5, func(string, int) bool {
		listenCalls++
		return true
	})
	actual, err := allocator.resolve(
		context.Background(),
		"db:3",
		[]localService{{name: "db", guestPort: 4, defaultHostPort: 4}},
	)
	if err != nil {
		t.Fatalf("expected explicit mapping to succeed, got %v", err)
	}
	if actual != "db:3" {
		t.Fatalf("expected explicit mapping to be preserved, got %q", actual)
	}
	if listenCalls != 0 {
		t.Fatalf("expected explicit mapping to bypass allocation, got %d listen calls", listenCalls)
	}
}

func TestLocalPortAllocator_NormalizesAutomaticMappings(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{"", "auto", "db:0"} {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			allocator := testLocalPortAllocator(t, 2, 5, func(_ string, port int) bool {
				return port == 4
			})
			actual, err := allocator.resolve(
				context.Background(),
				raw,
				[]localService{{name: "db", guestPort: 4, defaultHostPort: 4}},
			)
			if err != nil {
				t.Fatalf("expected %q to normalize, got %v", raw, err)
			}
			if actual != "db:4" {
				t.Fatalf("expected db:4, got %q", actual)
			}
		})
	}
}
