package tcpscan

import (
	"net"
	"strings"
	"testing"
)

const testProcRoutes = `Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT
eth0\t00000000\t010012AC\t0003\t0\t0\t100\t00000000\t0\t0\t0
eth0\t000012AC\t00000000\t0001\t0\t0\t0\t0000FFFF\t0\t0\t0
lan0\t001EA8C0\t00000000\t0001\t0\t0\t0\t0000FFFF\t0\t0\t0
`

func TestSelectProcRouteUsesGatewayForCrossSubnetTarget(t *testing.T) {
	routes, err := parseProcRoutes(strings.NewReader(strings.ReplaceAll(testProcRoutes, `\t`, "\t")))
	if err != nil {
		t.Fatal(err)
	}
	target := mustIPv4(t, "10.20.30.34")
	route, err := selectProcRoute(routes, target, "")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := route.iface, "eth0"; got != want {
		t.Fatalf("interface = %q, want %q", got, want)
	}
	if got, want := routeNextHop(route, target).String(), "172.18.0.1"; got != want {
		t.Fatalf("next hop = %s, want %s", got, want)
	}
}

func TestSelectProcRouteUsesTargetForDirectRoute(t *testing.T) {
	routes, err := parseProcRoutes(strings.NewReader(strings.ReplaceAll(testProcRoutes, `\t`, "\t")))
	if err != nil {
		t.Fatal(err)
	}
	target := mustIPv4(t, "192.168.30.34")
	route, err := selectProcRoute(routes, target, "lan0")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := routeNextHop(route, target).String(), target.String(); got != want {
		t.Fatalf("next hop = %s, want %s", got, want)
	}
}

func TestSelectProcRouteHonorsPreferredInterface(t *testing.T) {
	routes, err := parseProcRoutes(strings.NewReader(strings.ReplaceAll(testProcRoutes, `\t`, "\t")))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := selectProcRoute(routes, mustIPv4(t, "8.8.8.8"), "lan0"); err == nil {
		t.Fatal("expected no route on lan0")
	}
}

func mustIPv4(t *testing.T, value string) net.IP {
	t.Helper()
	ip := net.ParseIP(value).To4()
	if ip == nil {
		t.Fatalf("invalid test IP %q", value)
	}
	return ip
}
