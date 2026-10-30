// Command mdnsprobe compares hashicorp/mdns and brutella/dnssd for EdgeSync
// discovery (P0-SP5-003). It announces or browses _edgesync._udp on the local
// link and prints timestamped results measured from a monotonic clock.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/brutella/dnssd"
	"github.com/hashicorp/mdns"
)

const (
	libHashicorp = "hashicorp"
	libDNSSD     = "dnssd"

	modeAnnounce = "announce"
	modeBrowse   = "browse"

	serviceType     = "_edgesync._udp"
	serviceDomain   = "local"
	protocolVersion = "1"
	defaultPort     = 24860
)

type options struct {
	lib    string
	mode   string
	name   string
	host   string
	id     string
	role   string
	port   int
	tcp    int
	ifaces []string
	ipv4   bool
	ipv6   bool
	window time.Duration
}

type printer struct {
	out   io.Writer
	start time.Time
}

func (p printer) elapsed() time.Duration {
	return time.Since(p.start)
}

func (p printer) printf(format string, args ...any) {
	fmt.Fprintf(p.out, "%9.3fs %s", p.elapsed().Seconds(), fmt.Sprintf(format, args...))
}

type tracker struct {
	found int
	first time.Duration
}

func (t *tracker) record(elapsed time.Duration) {
	if t.found == 0 {
		t.first = elapsed
	}
	t.found++
}

func (t tracker) summary(p printer, lib string) {
	first := "none"
	if t.found > 0 {
		first = fmt.Sprintf("%.3fs", t.first.Seconds())
	}
	p.printf("SUMMARY lib=%s found=%d first_discovery=%s\n", lib, t.found, first)
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:], os.Stdout)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "mdnsprobe:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, out io.Writer) error {
	o, err := parseOptions(args)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := o.complete(); err != nil {
		return err
	}
	selected, err := selectInterfaces(o)
	if err != nil {
		return err
	}

	p := printer{out: out, start: time.Now()}
	p.printf("START lib=%s mode=%s service=%s.%s. name=%q host=%q id=%s role=%s port=%d tcp=%d goos=%s\n",
		o.lib, o.mode, serviceType, serviceDomain, o.name, o.host, o.id, o.role, o.port, o.tcp, runtime.GOOS)
	if err := printInterfaces(p, o, selected); err != nil {
		return err
	}

	var tr tracker
	switch {
	case o.lib == libHashicorp && o.mode == modeAnnounce:
		err = announceHashicorp(ctx, p, o, selected)
	case o.lib == libHashicorp && o.mode == modeBrowse:
		err = browseHashicorp(ctx, p, &tr, o, selected)
	case o.lib == libDNSSD && o.mode == modeAnnounce:
		err = announceDNSSD(ctx, p, o)
	case o.lib == libDNSSD && o.mode == modeBrowse:
		err = browseDNSSD(ctx, p, &tr)
	default:
		return fmt.Errorf("unsupported lib %q mode %q", o.lib, o.mode)
	}
	if err != nil {
		return err
	}
	if o.mode == modeBrowse {
		tr.summary(p, o.lib)
	}
	return nil
}

func parseOptions(args []string) (options, error) {
	var o options
	var ifaces string
	fs := flag.NewFlagSet("mdnsprobe", flag.ContinueOnError)
	fs.StringVar(&o.lib, "lib", "", "library under test: hashicorp or dnssd")
	fs.StringVar(&o.mode, "mode", "", "announce or browse")
	fs.StringVar(&o.name, "name", "", "instance name for announce (default: host name)")
	fs.StringVar(&o.id, "id", "", "TXT id value (default: random 8 bytes, hex)")
	fs.StringVar(&o.role, "role", "client", "TXT role value: server or client")
	fs.IntVar(&o.port, "port", defaultPort, "port advertised for announce")
	fs.IntVar(&o.tcp, "tcp", defaultPort, "TXT tcp value")
	fs.StringVar(&ifaces, "ifaces", "", "comma-separated interface names (default: library default)")
	fs.BoolVar(&o.ipv4, "ipv4", true, "hashicorp browse only: query over IPv4")
	fs.BoolVar(&o.ipv6, "ipv6", true, "hashicorp browse only: query over IPv6")
	fs.DurationVar(&o.window, "window", 3*time.Second, "hashicorp browse only: length of one query round")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if fs.NArg() != 0 {
		return options{}, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if ifaces != "" {
		for _, name := range strings.Split(ifaces, ",") {
			o.ifaces = append(o.ifaces, strings.TrimSpace(name))
		}
	}
	if err := o.validate(); err != nil {
		return options{}, err
	}
	return o, nil
}

func (o options) validate() error {
	switch o.lib {
	case libHashicorp, libDNSSD:
	default:
		return fmt.Errorf("-lib must be %s or %s, got %q", libHashicorp, libDNSSD, o.lib)
	}
	switch o.mode {
	case modeAnnounce, modeBrowse:
	default:
		return fmt.Errorf("-mode must be %s or %s, got %q", modeAnnounce, modeBrowse, o.mode)
	}
	if o.role != "server" && o.role != "client" {
		return fmt.Errorf("-role must be server or client, got %q", o.role)
	}
	if o.port < 1 || o.port > 65535 {
		return fmt.Errorf("-port out of range: %d", o.port)
	}
	if o.tcp < 1 || o.tcp > 65535 {
		return fmt.Errorf("-tcp out of range: %d", o.tcp)
	}
	if o.window <= 0 {
		return fmt.Errorf("-window must be positive, got %s", o.window)
	}
	if o.lib == libHashicorp && len(o.ifaces) > 1 {
		return errors.New("-lib=hashicorp takes one interface: its Config.Iface and QueryParam.Interface are single values")
	}
	if o.lib == libDNSSD && o.mode == modeBrowse && len(o.ifaces) > 0 {
		return errors.New("-lib=dnssd browse has no interface selection: LookupType queries every multicast interface")
	}
	if (o.lib == libDNSSD || o.mode == modeAnnounce) && (!o.ipv4 || !o.ipv6) {
		return errors.New("-ipv4 and -ipv6 apply to -lib=hashicorp -mode=browse only")
	}
	return nil
}

func (o *options) complete() error {
	host, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("host name: %w", err)
	}
	o.host = host
	if o.name == "" {
		o.name = host
	}
	if o.id == "" {
		raw := make([]byte, 8)
		if _, err := rand.Read(raw); err != nil {
			return fmt.Errorf("random id: %w", err)
		}
		o.id = hex.EncodeToString(raw)
	}
	return nil
}

func selectInterfaces(o options) ([]*net.Interface, error) {
	if len(o.ifaces) == 0 {
		if o.lib != libDNSSD {
			return nil, nil
		}
		ifis := dnssd.MulticastInterfaces()
		if len(ifis) == 0 {
			return nil, errors.New("no multicast interface with an address")
		}
		return ifis, nil
	}
	ifis := make([]*net.Interface, 0, len(o.ifaces))
	for _, name := range o.ifaces {
		ifi, err := net.InterfaceByName(name)
		if err != nil {
			return nil, fmt.Errorf("interface %q: %w", name, err)
		}
		ifis = append(ifis, ifi)
	}
	return ifis, nil
}

func printInterfaces(p printer, o options, selected []*net.Interface) error {
	all, err := net.Interfaces()
	if err != nil {
		return fmt.Errorf("list interfaces: %w", err)
	}
	for _, ifi := range all {
		addrs, err := ifi.Addrs()
		if err != nil {
			return fmt.Errorf("addresses of %s: %w", ifi.Name, err)
		}
		p.printf("IFACE index=%d name=%s flags=%s addrs=%v bound=%t\n",
			ifi.Index, ifi.Name, ifi.Flags, addrs, isSelected(selected, ifi))
	}
	if o.lib == libHashicorp && len(selected) == 0 {
		p.printf("BIND hashicorp: system default multicast interface (the library does not report which one)\n")
	}
	for _, ifi := range selected {
		addrs, err := ifi.Addrs()
		if err != nil {
			return fmt.Errorf("addresses of %s: %w", ifi.Name, err)
		}
		p.printf("BIND %s index=%d addrs=%v\n", ifi.Name, ifi.Index, addrs)
	}
	return nil
}

func isSelected(selected []*net.Interface, ifi net.Interface) bool {
	for _, s := range selected {
		if s.Index == ifi.Index {
			return true
		}
	}
	return false
}

func hashicorpTXT(o options) []string {
	return []string{
		"v=" + protocolVersion,
		"id=" + o.id,
		"role=" + o.role,
		"tcp=" + strconv.Itoa(o.tcp),
	}
}

func dnssdTXT(o options) map[string]string {
	return map[string]string{
		"v":    protocolVersion,
		"id":   o.id,
		"role": o.role,
		"tcp":  strconv.Itoa(o.tcp),
	}
}

func announceHashicorp(ctx context.Context, p printer, o options, selected []*net.Interface) error {
	ips, err := advertisedIPs(selected)
	if err != nil {
		return err
	}
	svc, err := mdns.NewMDNSService(o.name, serviceType, serviceDomain+".", o.host+"."+serviceDomain+".", o.port, ips, hashicorpTXT(o))
	if err != nil {
		return fmt.Errorf("hashicorp service: %w", err)
	}
	cfg := mdns.Config{
		Zone:   svc,
		Logger: log.New(os.Stderr, "hashicorp/mdns ", log.Lmicroseconds),
	}
	if len(selected) == 1 {
		cfg.Iface = selected[0]
	}
	srv, err := mdns.NewServer(&cfg)
	if err != nil {
		return fmt.Errorf("hashicorp server: %w", err)
	}
	p.printf("ANNOUNCE hashicorp instance=%q ips=%v port=%d (answers queries only, no unsolicited announcement)\n", o.name, ips, o.port)
	<-ctx.Done()
	if err := srv.Shutdown(); err != nil {
		return fmt.Errorf("hashicorp shutdown: %w", err)
	}
	p.printf("SHUTDOWN hashicorp sockets closed, no goodbye packet (the library has no goodbye API)\n")
	return nil
}

func advertisedIPs(selected []*net.Interface) ([]net.IP, error) {
	ifis := selected
	if len(ifis) == 0 {
		all, err := net.Interfaces()
		if err != nil {
			return nil, fmt.Errorf("list interfaces: %w", err)
		}
		for i := range all {
			ifis = append(ifis, &all[i])
		}
	}
	var ips []net.IP
	for _, ifi := range ifis {
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifi.Addrs()
		if err != nil {
			return nil, fmt.Errorf("addresses of %s: %w", ifi.Name, err)
		}
		for _, a := range addrs {
			ipNet, ok := a.(*net.IPNet)
			if !ok || ipNet.IP.IsLinkLocalUnicast() {
				continue
			}
			ips = append(ips, ipNet.IP)
		}
	}
	if len(ips) == 0 {
		return nil, errors.New("no advertisable unicast address on the selected interfaces")
	}
	return ips, nil
}

func browseHashicorp(ctx context.Context, p printer, tr *tracker, o options, selected []*net.Interface) error {
	entries := make(chan *mdns.ServiceEntry, 64)
	loopErr := make(chan error, 1)
	go func() {
		loopErr <- queryRounds(ctx, o, selected, entries)
	}()
	seen := make(map[string]string)
	for {
		select {
		case e := <-entries:
			if err := reportHashicorp(p, tr, e, seen); err != nil {
				return err
			}
		case err := <-loopErr:
			return err
		}
	}
}

func queryRounds(ctx context.Context, o options, selected []*net.Interface, entries chan<- *mdns.ServiceEntry) error {
	for ctx.Err() == nil {
		params := mdns.DefaultParams(serviceType)
		params.Entries = entries
		params.Timeout = o.window
		params.DisableIPv4 = !o.ipv4
		params.DisableIPv6 = !o.ipv6
		params.Logger = log.New(os.Stderr, "hashicorp/mdns ", log.Lmicroseconds)
		if len(selected) == 1 {
			params.Interface = selected[0]
		}
		if err := mdns.QueryContext(ctx, params); err != nil {
			return fmt.Errorf("hashicorp query: %w", err)
		}
	}
	return nil
}

func reportHashicorp(p printer, tr *tracker, e *mdns.ServiceEntry, seen map[string]string) error {
	sig := fmt.Sprintf("%s|%v|%v|%d|%q", e.Host, e.AddrV4, e.AddrV6, e.Port, e.InfoFields)
	prev, known := seen[e.Name]
	if known && prev == sig {
		return nil
	}
	seen[e.Name] = sig

	addr := e.AddrV4
	if addr == nil {
		addr = e.AddrV6
	}
	ifaceNames := "none"
	if addr != nil {
		names, err := interfacesContaining(addr)
		if err != nil {
			return err
		}
		if len(names) > 0 {
			ifaceNames = strings.Join(names, ",")
		}
	}
	zone := ""
	if e.AddrV6IPAddr != nil {
		zone = e.AddrV6IPAddr.Zone
	}

	verb := "FOUND"
	if known {
		verb = "UPDATE"
	} else {
		tr.record(p.elapsed())
	}
	p.printf("%s hashicorp name=%q host=%s port=%d v4=%v v6=%v iface-by-subnet=%s v6zone=%q txt=%q\n",
		verb, e.Name, e.Host, e.Port, e.AddrV4, e.AddrV6, ifaceNames, zone, e.InfoFields)
	return nil
}

func interfacesContaining(ip net.IP) ([]string, error) {
	all, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("list interfaces: %w", err)
	}
	var names []string
	for _, ifi := range all {
		addrs, err := ifi.Addrs()
		if err != nil {
			return nil, fmt.Errorf("addresses of %s: %w", ifi.Name, err)
		}
		for _, a := range addrs {
			if ipNet, ok := a.(*net.IPNet); ok && ipNet.Contains(ip) {
				names = append(names, ifi.Name)
				break
			}
		}
	}
	return names, nil
}

func announceDNSSD(ctx context.Context, p printer, o options) error {
	svc, err := dnssd.NewService(dnssd.Config{
		Name:   o.name,
		Type:   serviceType,
		Domain: serviceDomain,
		Host:   o.host,
		Text:   dnssdTXT(o),
		Port:   o.port,
		Ifaces: o.ifaces,
	})
	if err != nil {
		return fmt.Errorf("dnssd service: %w", err)
	}
	rp, err := dnssd.NewResponder()
	if err != nil {
		return fmt.Errorf("dnssd responder: %w", err)
	}
	hdl, err := rp.Add(svc)
	if err != nil {
		return fmt.Errorf("dnssd add: %w", err)
	}
	p.printf("ANNOUNCE dnssd name=%q port=%d (probing, then announcing on each interface)\n", o.name, o.port)
	if err := rp.Respond(ctx); !errors.Is(err, context.Canceled) {
		return fmt.Errorf("dnssd respond: %w", err)
	}
	final := hdl.Service()
	p.printf("SHUTDOWN dnssd goodbye issued, final name=%q port=%d\n", final.Name, final.Port)
	return nil
}

func browseDNSSD(ctx context.Context, p printer, tr *tracker) error {
	err := dnssd.LookupType(ctx, serviceType+"."+serviceDomain+".",
		func(e dnssd.BrowseEntry) {
			tr.record(p.elapsed())
			p.printf("FOUND dnssd name=%q host=%s port=%d iface=%s ips=%v txt=%v\n",
				e.Name, e.Host, e.Port, e.IfaceName, e.IPs, e.Text)
		},
		func(e dnssd.BrowseEntry) {
			p.printf("GONE dnssd name=%q iface=%s\n", e.Name, e.IfaceName)
		},
	)
	if !errors.Is(err, context.Canceled) {
		return fmt.Errorf("dnssd browse: %w", err)
	}
	return nil
}
