// LatchAdmin: a web panel to install and run MafiaHub game servers and keep their Latch scripts up to date.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/heylatch/latchadmin/internal/app"
	"github.com/heylatch/latchadmin/internal/buildinfo"
	"github.com/heylatch/latchadmin/internal/selfupdate"
	"github.com/heylatch/latchadmin/internal/web"
	"github.com/heylatch/latchadmin/ui"
)

func main() {
	exeDir := "."
	if exe, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exe)
	}
	dataDir := flag.String("data", filepath.Join(exeDir, "latchadmin-data"), "folder for LatchAdmin's settings, users and logs")
	host := flag.String("host", "0.0.0.0", "address to listen on (127.0.0.1 = this machine only)")
	port := flag.Int("port", 40120, "port for the web panel")
	cert := flag.String("tls-cert", "", "TLS certificate file (serve HTTPS)")
	key := flag.String("tls-key", "", "TLS key file")
	proxies := flag.String("trusted-proxy", "", "reverse proxy addresses or CIDR ranges whose X-Forwarded-For/-Proto headers are trusted, comma-separated")
	version := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if err := web.TrustProxies(*proxies); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if *version {
		fmt.Println(buildinfo.Version)
		return
	}

	selfupdate.CleanOld()
	a, err := app.New(*dataDir)
	if err != nil {
		log.Fatalf("LatchAdmin can't start: %v", err)
	}
	srv := web.New(a, ui.FS())
	interactive := setupTerminal()
	color = (interactive || os.Getenv("FORCE_COLOR") != "") && os.Getenv("NO_COLOR") == ""

	addr := net.JoinHostPort(*host, strconv.Itoa(*port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("Can't listen on %s: %v (is LatchAdmin already running?)", addr, err)
	}
	scheme := "http"
	if *cert != "" {
		scheme = "https"
	}
	links := urls(scheme, *host, *port)
	printBanner(buildinfo.Version, *dataDir, links, scheme == "http" && *host != "127.0.0.1" && *host != "localhost")
	// The setup link carries the PIN, so the page opens with it filled in. It's printed only here, on the
	// machine's own console, and the page drops it from the address bar right away.
	setupLink := func(pin string) string { return links[0] + "/?pin=" + pin }
	srv.OnPIN = func(pin string) { printSetup(pin, setupLink(pin), false) }
	if a.NeedsSetup() {
		pin := a.PIN.New()
		printSetup(pin, setupLink(pin), interactive)
		if interactive {
			openBrowser(setupLink(pin))
		}
	} else {
		fmt.Println()
	}

	ctx, stop := context.WithCancel(context.Background())
	a.Restart = func(exe string) {
		_ = ln.Close()
		if err := selfupdate.Restart(exe); err != nil {
			log.Printf("restart after update failed: %v; start LatchAdmin again", err)
			os.Exit(1)
		}
	}
	go a.Run(ctx)
	go a.Boot()

	hs := &http.Server{Handler: srv, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second}
	go func() {
		var err error
		if *cert != "" {
			err = hs.ServeTLS(ln, *cert, *key)
		} else {
			err = hs.Serve(ln)
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			log.Fatalf("web server: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	fmt.Println("\n  Stopping game servers…")
	stop()
	sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_ = hs.Shutdown(sctx)
	cancel()
	a.Shutdown()
}

// urls lists addresses to open the panel at.
func urls(scheme, host string, port int) []string {
	p := strconv.Itoa(port)
	if host != "0.0.0.0" && host != "::" && host != "" {
		return []string{scheme + "://" + net.JoinHostPort(host, p)}
	}
	out := []string{scheme + "://localhost:" + p}
	ifaces, _ := net.InterfaceAddrs()
	for _, ad := range ifaces {
		if ipn, ok := ad.(*net.IPNet); ok && ipn.IP.To4() != nil && !ipn.IP.IsLoopback() && !ipn.IP.IsLinkLocalUnicast() {
			out = append(out, scheme+"://"+net.JoinHostPort(ipn.IP.String(), p))
		}
	}
	return out
}
