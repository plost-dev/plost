package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/plost-dev/plost/internal/plost"
)

var version = "0.1.0-dev"

func main() {
	addr := flag.String("listen", "127.0.0.1:3101", "HTTP listening address")
	data := flag.String("data", "./data", "Private data directory")
	origin := flag.String("origin", "", "Public origin, e.g. https://stats.example.com")
	proxy := flag.Bool("trust-proxy", false, "Trust a reverse proxy on this machine (same as -trusted-proxies loopback)")
	trusted := flag.String("trusted-proxies", "", "Comma-separated proxies and CDNs whose forwarding headers count: loopback, private (Docker, LAN), cloudflare, networks such as 203.0.113.0/24, or @file with one per line")
	ipHeader := flag.String("client-ip-header", "", "Header a trusted proxy sets to the visitor's address alone, e.g. CF-Connecting-IP or Ali-CDN-Real-IP; by default X-Forwarded-For is read")
	geoip := flag.String("geoip-db", "", "MaxMind-format database (GeoLite2 City or Country, DB-IP) used to tell visitors' countries when no CDN header does")
	showVersion := flag.Bool("version", false, "Print version")
	memory := flag.Int("memory", 64, "Go runtime soft memory limit in MiB (not a total process limit)")
	allowOutbound := flag.String("allow-outbound", "", "Comma-separated networks that site checks and cloud backups may connect to although they are normally refused, e.g. 198.18.0.0/15 for a local proxy's fake-IP mode")
	flag.Parse()
	if *showVersion {
		fmt.Println("Plost", version)
		return
	}
	if *memory > 0 {
		debug.SetMemoryLimit(int64(*memory) << 20)
	}
	var allow []netip.Prefix
	for _, field := range strings.Split(*allowOutbound, ",") {
		if field = strings.TrimSpace(field); field == "" {
			continue
		}
		network, err := netip.ParsePrefix(field)
		if err != nil {
			log.Fatalf("-allow-outbound: %q is not a network such as 198.18.0.0/15", field)
		}
		allow = append(allow, network)
	}
	trustedNets, trustedNames, err := plost.ParseTrustedProxies(*trusted)
	if err != nil {
		log.Fatal("-trusted-proxies: ", err)
	}
	header, err := plost.CanonicalClientIPHeader(*ipHeader)
	if err != nil {
		log.Fatal("-", err)
	}
	if header != "" && !*proxy && len(trustedNets) == 0 {
		log.Fatal("-client-ip-header only applies behind a trusted proxy; add -trusted-proxies")
	}
	app, err := plost.Open(plost.Config{DataDir: *data, Origin: *origin, TrustProxy: *proxy, TrustedProxies: trustedNets, TrustedProxyNames: trustedNames, ClientIPHeader: header, GeoIPPath: *geoip, Version: version, OutboundAllow: allow})
	if err != nil {
		log.Fatal("Cannot open Plost: ", err)
	}
	defer app.Close()
	srv := &http.Server{Addr: *addr, Handler: app.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	log.Printf("Plost %s listening on %s", version, *addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal("HTTP server: ", err)
	}
}
