package ratelimit

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// ClientIP is the address a request came from: CF-Connecting-IP when the TCP peer is a trusted
// proxy (the tunnel connector), else the peer itself. A header from anyone else is ignored,
// so a client cannot choose its own bucket.
func ClientIP(r *http.Request, trusted []netip.Prefix) string {
	peer, ok := peerAddr(r)
	if !ok {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		return host
	}
	if isTrusted(peer, trusted) {
		if cf, err := netip.ParseAddr(strings.TrimSpace(r.Header.Get("CF-Connecting-IP"))); err == nil {
			return cf.Unmap().String()
		}
	}
	return peer.String()
}

// ViaEdge reports whether the request arrived through the public edge: its TCP peer is a
// trusted proxy and it carries a CF-Connecting-IP header.
func ViaEdge(r *http.Request, trusted []netip.Prefix) bool {
	peer, ok := peerAddr(r)
	return ok && isTrusted(peer, trusted) && r.Header.Get("CF-Connecting-IP") != ""
}

func peerAddr(r *http.Request) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, false
	}
	return peer.Unmap(), true
}

func isTrusted(peer netip.Addr, trusted []netip.Prefix) bool {
	for _, p := range trusted {
		if p.Contains(peer) {
			return true
		}
	}
	return false
}

// HostPrefixes is the default trust: every address of this host's own interfaces, which is
// where a tunnel connector on the same host connects from.
func HostPrefixes() ([]netip.Prefix, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	var out []netip.Prefix
	for _, a := range addrs {
		ipn, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip, ok := netip.AddrFromSlice(ipn.IP)
		if !ok {
			continue
		}
		ip = ip.Unmap()
		out = append(out, netip.PrefixFrom(ip, ip.BitLen()))
	}
	return out, nil
}
