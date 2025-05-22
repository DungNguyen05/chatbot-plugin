// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/mattermost/mattermost/server/public/shared/httpservice"
)

// Hostname matching rules:
// 1. Exact matches: A hostname must exactly match an allowed pattern
// 2. Wildcard matches: Patterns starting with "*." only match subdomains
//    - "*.example.com" matches "sub.example.com" and "deep.sub.example.com"
//    - "*.example.com" does NOT match "example.com" itself
// 3. Global wildcard: A pattern of "*" matches all hostnames
// 4. IPv6 zones: Hostnames containing zone IDs (%) require exact matches
//    - Wildcard patterns never match hostnames containing zone IDs

// hostnameAllowed checks if a hostname matches any of the allowed patterns
func hostnameAllowed(hostname string, allowedPatterns []string) bool {
	for _, pattern := range allowedPatterns {
		if pattern == "*" {
			return true
		}

		if strings.HasPrefix(pattern, "*.") {
			// Reject hosts with ipv6 zones
			if strings.ContainsAny(hostname, "%") {
				return false
			}

			suffix := pattern[1:] // Remove the *
			if strings.HasSuffix(hostname, suffix) {
				return true
			}
		} else if hostname == pattern {
			return true
		}
	}
	return false
}

// parseAllowedHostnames splits the comma-separated string into cleaned hostname patterns
func parseAllowedHostnames(allowedHostnames string) []string {
	allowedHostnames = strings.TrimSpace(allowedHostnames)
	if allowedHostnames == "" {
		return nil
	}

	var patterns []string
	userPatterns := strings.Split(allowedHostnames, ",")
	for _, p := range userPatterns {
		p = strings.TrimSpace(p)
		if p != "" {
			patterns = append(patterns, p)
		}
	}

	return patterns
}

// restrictedTransport wraps an http.RoundTripper to enforce hostname restrictions
type restrictedTransport struct {
	wrapped      http.RoundTripper
	allowedHosts []string
}

func (t *restrictedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL == nil {
		return nil, fmt.Errorf("request has no URL")
	}

	hostname := req.URL.Hostname()
	if !hostnameAllowed(hostname, t.allowedHosts) {
		return nil, fmt.Errorf("hostname %q is not on allowed list, add this host to allowed upstream hosts", hostname)
	}

	// Add CORS headers to outgoing requests
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	req.Header.Set("Access-Control-Allow-Origin", "*")
	req.Header.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	req.Header.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	return t.wrapped.RoundTrip(req)
}

// wrapTransportWithHostRestrictions wraps an existing transport with hostname restrictions
func wrapTransportWithHostRestrictions(base http.RoundTripper, allowedHostnames []string) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}

	return &restrictedTransport{
		wrapped:      base,
		allowedHosts: allowedHostnames,
	}
}

// createRestrictedClient creates an http.Client with hostname restrictions
func createRestrictedClient(client *http.Client, allowedHostnames []string) *http.Client {
	if client == nil {
		client = &http.Client{}
	}

	// Wrap the existing transport or create new one
	client.Transport = wrapTransportWithHostRestrictions(client.Transport, allowedHostnames)

	return client
}

func (p *Plugin) createExternalHTTPClient() *http.Client {
	baseClient := httpservice.MakeHTTPServicePlugin(p.API).MakeClient(false)
	config := p.getConfiguration()

	// Add ERP domain to allowed hostnames if configured
	var allowedHosts []string
	if config.RollCall.ERPDomain != "" {
		// Extract hostname from ERP domain
		erpHost := extractHostname(config.RollCall.ERPDomain)
		// Add to allowed hostnames
		if erpHost != "" {
			allowedHosts = append([]string{erpHost}, parseAllowedHostnames(config.AllowedUpstreamHostnames)...)
		} else {
			allowedHosts = parseAllowedHostnames(config.AllowedUpstreamHostnames)
		}
	} else {
		allowedHosts = parseAllowedHostnames(config.AllowedUpstreamHostnames)
	}

	return createRestrictedClient(baseClient, allowedHosts)
}

// extractHostname extracts the hostname from a URL
func extractHostname(urlStr string) string {
	if !strings.HasPrefix(urlStr, "http://") && !strings.HasPrefix(urlStr, "https://") {
		urlStr = "https://" + urlStr
	}

	// Simple extraction without using url.Parse to avoid potential errors
	host := strings.TrimPrefix(urlStr, "http://")
	host = strings.TrimPrefix(host, "https://")
	host = strings.Split(host, "/")[0]
	host = strings.Split(host, ":")[0] // Remove port if present

	return host
}
