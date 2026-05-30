package services

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
)

// secureWebhookTransport returns an http.RoundTripper that:
//   - Always verifies TLS certificates (InsecureSkipVerify is never set).
//   - Re-validates the resolved IP at dial time to prevent DNS rebinding / SSRF.
//     This checks the IP again after DNS resolution, defeating TOCTOU attacks
//     where a hostname passes validation but later resolves to a private address.
func secureWebhookTransport() http.RoundTripper {
	return &http.Transport{
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupHost(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("DNS resolution failed: %w", err)
			}
			for _, ipStr := range ips {
				ip := net.ParseIP(ipStr)
				if ip == nil {
					continue
				}
				if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
					ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
					return nil, fmt.Errorf("webhook target resolves to a disallowed IP: %s", ipStr)
				}
			}
			if len(ips) == 0 {
				return nil, fmt.Errorf("no IP addresses for host %q", host)
			}
			return (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(ips[0], port))
		},
	}
}
