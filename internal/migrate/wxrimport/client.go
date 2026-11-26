package wxrimport

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"
)

// errPrivateAddress refuses downloads that would reach the server's own
// network: a crafted export file could otherwise make the importer probe
// the database, the admin or cloud metadata endpoints.
var errPrivateAddress = errors.New("社内向けのアドレスには接続しません")

// publicClient downloads only from public addresses. The check runs on the
// resolved address at connect time, so DNS answers cannot slip past it.
func publicClient() *http.Client {
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip, err := netip.ParseAddr(host)
			if err != nil || !publicAddr(ip) {
				return fmt.Errorf("%w: %s", errPrivateAddress, host)
			}
			return nil
		},
	}
	transport := &http.Transport{
		Proxy:               nil,
		DialContext:         dialer.DialContext,
		TLSHandshakeTimeout: 10 * time.Second,
		MaxIdleConns:        10,
	}
	return &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("リダイレクトが多すぎます")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return fmt.Errorf("http(s) 以外へのリダイレクトです: %s", req.URL.Scheme)
			}
			return nil
		},
	}
}

func publicAddr(ip netip.Addr) bool {
	ip = ip.Unmap()
	return ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() &&
		!cgnat.Contains(ip) && !ip.IsUnspecified()
}

// cgnat is shared address space (RFC 6598), not reachable from outside.
var cgnat = netip.MustParsePrefix("100.64.0.0/10")
