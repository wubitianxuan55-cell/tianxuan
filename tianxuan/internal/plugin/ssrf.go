package plugin

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

// ssrfGuardedHTTPClient returns an http.Client whose dialer refuses connections
// to private, link-local, unspecified, and CGNAT addresses — the same surface a
// prompt-injected MCP server URL would aim at (cloud metadata, internal services).
// Loopback is allowed (the agent can already reach localhost via bash).
// The check runs at dial time on the resolved IP, so DNS rebinding is caught.
func ssrfGuardedHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 30 * time.Second}
	return &http.Client{
		Timeout: 60 * time.Second, // V8.2: 全请求超时防止 MCP 服务器无响应导致永久阻塞
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				host, port, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, err
				}
				ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
				if err != nil {
					return nil, err
				}
				for _, ip := range ips {
					if blockedPluginIP(ip.IP) {
						return nil, fmt.Errorf("plugin: refusing to connect to internal address %s (resolves to %s)", host, ip.IP)
					}
				}
				return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
			},
		},
	}
}

// cgnatRangePlugin is RFC 6598 shared address space (100.64.0.0/10). Go's
// IsPrivate doesn't cover it, yet some clouds host instance metadata there.
var cgnatRangePlugin = mustCIDR("100.64.0.0/10")

func mustCIDR(s string) *net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return n
}

// blockedPluginIP reports whether ip is an address the MCP plugin transport
// must not connect to.
func blockedPluginIP(ip net.IP) bool {
	return ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() ||
		cgnatRangePlugin.Contains(ip)
}
