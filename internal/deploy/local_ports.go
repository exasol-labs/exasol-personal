// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"strconv"
	"strings"
)

const (
	localMinimumAutomaticPort = 1024
	localMaximumPort          = 65535
	localDatabasePort         = 8563
	localDatabaseService      = "db"
)

type localService struct {
	name            string
	guestPort       int
	defaultHostPort int
}

var localServiceCatalog = []localService{{
	name:            localDatabaseService,
	guestPort:       localDatabasePort,
	defaultHostPort: localDatabasePort,
}}

type localPortAllocator struct {
	lookupIP    func(context.Context, string) ([]net.IPAddr, error)
	listen      func(string, string) (net.Listener, error)
	minimumPort int
	maximumPort int
}

func newLocalPortAllocator() localPortAllocator {
	resolver := net.DefaultResolver

	return localPortAllocator{
		lookupIP:    resolver.LookupIPAddr,
		listen:      net.Listen,
		minimumPort: localMinimumAutomaticPort,
		maximumPort: localMaximumPort,
	}
}

func (allocator localPortAllocator) resolve(
	ctx context.Context,
	raw string,
	services []localService,
) (string, error) {
	mappings, order, err := parseLocalPortMappings(raw)
	if err != nil {
		return "", err
	}

	reserved := make(map[int]string, len(mappings))
	for service, port := range mappings {
		if port <= 0 {
			continue
		}
		if other, exists := reserved[port]; exists && other != service {
			return "", fmt.Errorf(
				"local services %q and %q use the same host port %d",
				other,
				service,
				port,
			)
		}
		reserved[port] = service
	}

	var addresses []net.IPAddr
	held := make([]net.Listener, 0, len(services))
	defer func() {
		for _, listener := range held {
			_ = listener.Close()
		}
	}()

	known := make(map[string]struct{}, len(services))
	for _, service := range services {
		known[service.name] = struct{}{}
		port, configured := mappings[service.name]
		if configured && port > 0 {
			continue
		}
		if addresses == nil {
			addresses, err = allocator.loopbackAddresses(ctx)
			if err != nil {
				return "", err
			}
		}

		port, listeners, err := allocator.selectPort(service, addresses, reserved)
		if err != nil {
			return "", err
		}
		mappings[service.name] = port
		reserved[port] = service.name
		held = append(held, listeners...)
	}

	for service, port := range mappings {
		if _, exists := known[service]; !exists && port == 0 {
			return "", fmt.Errorf("cannot automatically allocate unknown local service %q", service)
		}
	}

	return formatLocalPortMappings(mappings, services, order), nil
}

func (allocator localPortAllocator) loopbackAddresses(ctx context.Context) ([]net.IPAddr, error) {
	addresses, err := allocator.lookupIP(ctx, "localhost")
	if err != nil {
		return nil, fmt.Errorf("failed to resolve localhost for local port allocation: %w", err)
	}

	seen := make(map[string]struct{}, len(addresses))
	loopbacks := make([]net.IPAddr, 0, len(addresses))
	for _, address := range addresses {
		if address.IP == nil || !address.IP.IsLoopback() {
			continue
		}
		key := address.IP.String() + "%" + address.Zone
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		loopbacks = append(loopbacks, address)
	}
	if len(loopbacks) == 0 {
		return nil, errors.New("localhost resolved to no loopback addresses")
	}

	return loopbacks, nil
}

func (allocator localPortAllocator) selectPort(
	service localService,
	addresses []net.IPAddr,
	reserved map[int]string,
) (int, []net.Listener, error) {
	if service.defaultHostPort < allocator.minimumPort ||
		service.defaultHostPort > allocator.maximumPort {
		return 0, nil, fmt.Errorf(
			"local service %q has invalid default host port %d",
			service.name,
			service.defaultHostPort,
		)
	}

	for port := service.defaultHostPort; port <= allocator.maximumPort; port++ {
		if listeners, ok := allocator.tryPort(port, addresses, reserved); ok {
			return port, listeners, nil
		}
	}
	for port := allocator.minimumPort; port < service.defaultHostPort; port++ {
		if listeners, ok := allocator.tryPort(port, addresses, reserved); ok {
			return port, listeners, nil
		}
	}

	return 0, nil, fmt.Errorf(
		"no usable host port is available for local service %q in range %d-%d",
		service.name,
		allocator.minimumPort,
		allocator.maximumPort,
	)
}

func (allocator localPortAllocator) tryPort(
	port int,
	addresses []net.IPAddr,
	reserved map[int]string,
) ([]net.Listener, bool) {
	if _, exists := reserved[port]; exists {
		return nil, false
	}

	listeners := make([]net.Listener, 0, len(addresses))
	for _, address := range addresses {
		host := address.IP.String()
		if address.Zone != "" {
			host += "%" + address.Zone
		}
		listener, err := allocator.listen(
			"tcp",
			net.JoinHostPort(host, strconv.Itoa(port)),
		)
		if err != nil {
			for _, opened := range listeners {
				_ = opened.Close()
			}

			return nil, false
		}
		listeners = append(listeners, listener)
	}

	return listeners, true
}

func parseLocalPortMappings(raw string) (map[string]int, []string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, "auto") {
		return map[string]int{}, nil, nil
	}

	mappings := make(map[string]int)
	order := make([]string, 0)
	for entry := range strings.SplitSeq(raw, ",") {
		entry = strings.TrimSpace(entry)
		service, rawPort, found := strings.Cut(entry, ":")
		service = strings.TrimSpace(service)
		rawPort = strings.TrimSpace(rawPort)
		if !found || service == "" || rawPort == "" {
			return nil, nil, fmt.Errorf(
				"invalid local port mapping %q; expected <service>:<port>",
				entry,
			)
		}
		if _, exists := mappings[service]; exists {
			return nil, nil, fmt.Errorf("local service %q is configured more than once", service)
		}
		port, err := strconv.Atoi(rawPort)
		if err != nil || port < 0 || port > localMaximumPort {
			return nil, nil, fmt.Errorf("invalid local port %q for service %q", rawPort, service)
		}
		mappings[service] = port
		order = append(order, service)
	}

	return mappings, order, nil
}

func formatLocalPortMappings(
	mappings map[string]int,
	services []localService,
	originalOrder []string,
) string {
	ordered := make([]string, 0, len(mappings))
	seen := make(map[string]struct{}, len(mappings))
	for _, service := range services {
		if _, exists := mappings[service.name]; !exists {
			continue
		}
		ordered = append(ordered, service.name)
		seen[service.name] = struct{}{}
	}
	for _, service := range originalOrder {
		if _, exists := seen[service]; exists {
			continue
		}
		ordered = append(ordered, service)
		seen[service] = struct{}{}
	}
	remaining := make([]string, 0, len(mappings)-len(ordered))
	for service := range mappings {
		if _, exists := seen[service]; !exists {
			remaining = append(remaining, service)
		}
	}
	slices.Sort(remaining)
	ordered = append(ordered, remaining...)

	parts := make([]string, 0, len(ordered))
	for _, service := range ordered {
		parts = append(parts, service+":"+strconv.Itoa(mappings[service]))
	}

	return strings.Join(parts, ",")
}
