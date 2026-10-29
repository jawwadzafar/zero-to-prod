package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"syscall"
	"time"
)

// When users type in their own webhook URL, your server will make requests
// to wherever they point it. Pointed at http://169.254.169.254/ (the cloud
// metadata service) or http://10.0.0.5:6379/ (your Redis), your server
// becomes the attacker's way into your private network: server-side
// request forgery (SSRF).

var ErrBlockedAddress = errors.New("destination is not a public internet address")

// ValidateURL is the cheap first check, at the moment a user saves a URL.
func ValidateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "https" {
		return errors.New("webhook URLs must use https")
	}
	if u.Hostname() == "" || u.User != nil {
		return errors.New("webhook URL needs a host and no user:password")
	}
	return nil
}

// isPublic reports whether ip is an ordinary internet address.
func isPublic(ip netip.Addr) bool {
	ip = ip.Unmap() // ::ffff:127.0.0.1 is 127.0.0.1 in disguise
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() || ip.IsInterfaceLocalMulticast() {
		return false
	}
	for _, p := range blockedPrefixes {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}

var blockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"),     // carrier-grade NAT, used inside some clouds
	netip.MustParsePrefix("192.0.0.0/24"),      // IETF protocol assignments
	netip.MustParsePrefix("198.18.0.0/15"),     // benchmarking networks
	netip.MustParsePrefix("240.0.0.0/4"),       // reserved
	netip.MustParsePrefix("64:ff9b::/96"),      // NAT64: can reach IPv4 private ranges
	netip.MustParsePrefix("fd00:ec2::254/128"), // AWS metadata over IPv6 (also inside fc00::/7)
}

// SafeClient returns an HTTP client for user-supplied URLs.
//
// The check runs in the dialer's Control hook, AFTER DNS has been resolved,
// on the exact IP about to be connected to. Checking the hostname when the
// URL is saved is not enough: DNS can answer "a public IP" at save time and
// "127.0.0.1" at send time (DNS rebinding).
func SafeClient() *http.Client {
	dialer := &net.Dialer{
		Timeout: 5 * time.Second,
		Control: func(network, address string, _ syscall.RawConn) error {
			ap, err := netip.ParseAddrPort(address)
			if err != nil {
				return err
			}
			if !isPublic(ap.Addr()) {
				return fmt.Errorf("%w: %s", ErrBlockedAddress, ap.Addr())
			}
			return nil
		},
	}
	return &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			Proxy:               nil, // never route through an environment proxy that might reach inside
			DialContext:         dialer.DialContext,
			TLSHandshakeTimeout: 5 * time.Second,
		},
		// Don't follow redirects: a public URL could redirect to an internal
		// one, and the Standard Webhooks spec says to treat 3xx as failure.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}
