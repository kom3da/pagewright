package wxrimport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
)

func TestPublicAddr(t *testing.T) {
	for addr, want := range map[string]bool{
		"8.8.8.8": true, "2001:4860:4860::8888": true,
		"127.0.0.1": false, "10.0.0.5": false, "172.16.3.4": false, "192.168.1.1": false,
		"169.254.169.254": false, "100.64.0.1": false, "0.0.0.0": false, "::1": false,
		"fe80::1": false, "fd00::1": false, "::ffff:127.0.0.1": false,
	} {
		if got := publicAddr(netip.MustParseAddr(addr)); got != want {
			t.Errorf("publicAddr(%s) = %v, want %v", addr, got, want)
		}
	}
}

func TestPublicClientRefusesLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()
	_, err := fetch(context.Background(), publicClient(), srv.URL+"/a.png")
	if !errors.Is(err, errPrivateAddress) {
		t.Fatalf("fetch from loopback: err = %v, want errPrivateAddress", err)
	}
}

func TestWriteFileMode(t *testing.T) {
	root := t.TempDir()
	if err := writeFile(root, "2026/01/a.png", []byte("x")); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(filepath.Join(root, "2026", "01", "a.png"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o640 {
		t.Errorf("mode = %o, want 640 so the web server can read it", got)
	}
}
