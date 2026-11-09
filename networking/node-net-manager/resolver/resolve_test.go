package resolver

import (
	"NetManager/TableEntryCache"
	"NetManager/events"
	mqttifce "NetManager/mqtt"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"
)

func resolvableEntry(job, vip string) TableEntryCache.TableEntry {
	return TableEntryCache.TableEntry{
		JobName:          job,
		Appname:          "app",
		Appns:            "ns",
		Servicename:      "svc",
		Servicenamespace: "svcns",
		Instancenumber:   0,
		Cluster:          0,
		Nodeip:           net.ParseIP("10.0.0.1"),
		Nodeport:         50103,
		Nsip:             net.ParseIP("10.19.1.1"),
		Nsipv6:           net.ParseIP("fc00::1"),
		ServiceIP: []TableEntryCache.ServiceIP{{
			IpType:     TableEntryCache.RoundRobin,
			Address:    net.ParseIP(vip),
			Address_v6: net.ParseIP("fdff::1"),
		}},
	}
}

// an unresolved query must return an error, or the negative cache never arms
// and every packet burst restarts a blocking MQTT round trip
func TestUnresolvedQueryIsAFailure(t *testing.T) {
	const vip = "10.30.9.9"
	addr := netip.MustParseAddr(vip)

	for name, query := range map[string]func(netip.Addr) ([]TableEntryCache.TableEntry, error){
		"query failed": func(netip.Addr) ([]TableEntryCache.TableEntry, error) {
			return nil, errors.New("mqtt timeout")
		},
		"empty response": func(netip.Addr) ([]TableEntryCache.TableEntry, error) {
			return nil, nil
		},
		"invalid entries": func(netip.Addr) ([]TableEntryCache.TableEntry, error) {
			bad := resolvableEntry("job", vip)
			bad.Nodeip = nil
			return []TableEntryCache.TableEntry{bad}, nil
		},
		"response for a different address": func(netip.Addr) ([]TableEntryCache.TableEntry, error) {
			return []TableEntryCache.TableEntry{resolvableEntry("job", "10.30.1.1")}, nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := &ServiceResolver{
				translationTable: TableEntryCache.NewTableManager(),
				tableQuery:       query,
			}

			if err := r.resolveServiceIP(addr); err == nil {
				t.Fatal("resolveServiceIP reported success for an address it did not resolve")
			}

			<-r.resolveServiceIPOnce(addr)
			r.resolveLock.Lock()
			_, remembered := r.failedServiceIPs[addr]
			r.resolveLock.Unlock()
			if !remembered {
				t.Error("the failed resolution was not recorded in the negative cache")
			}

			if again := r.resolveServiceIPOnce(addr); again != nil {
				t.Error("a second resolution was started inside the negative-cache TTL")
			}
		})
	}
}

func TestNegativeCacheExpires(t *testing.T) {
	addr := netip.MustParseAddr("10.30.9.9")
	r := &ServiceResolver{
		translationTable: TableEntryCache.NewTableManager(),
		tableQuery: func(netip.Addr) ([]TableEntryCache.TableEntry, error) {
			return nil, errors.New("mqtt timeout")
		},
	}

	<-r.resolveServiceIPOnce(addr)
	if r.resolveServiceIPOnce(addr) != nil {
		t.Fatal("expected the negative cache to suppress an immediate retry")
	}

	r.resolveLock.Lock()
	r.failedServiceIPs[addr] = time.Now().Add(-2 * negativeResolveCacheTTL)
	r.resolveLock.Unlock()

	done := r.resolveServiceIPOnce(addr)
	if done == nil {
		t.Fatal("expected a retry once the negative-cache TTL had passed")
	}
	<-done
}

// The proxy replay tests cover what happens after Resolving closes; this one
// covers the resolver side: sharing one in-flight query and installing the
// full response so later lookups hit the local cache.
func TestSuccessfulLookupInstallsAndReusesResolvedEntries(t *testing.T) {
	const (
		job = "app.ns.svc.svcns"
		vip = "10.30.9.9"
	)
	addr := netip.MustParseAddr(vip)
	events.Delete(job)
	t.Cleanup(func() { events.Delete(job) })

	firstEntry := resolvableEntry(job, vip)
	secondEntry := resolvableEntry(job, vip)
	secondEntry.Instancenumber = 1
	secondEntry.Nsip = net.ParseIP("10.19.1.2")
	secondEntry.Nsipv6 = net.ParseIP("fc00::2")

	queryStarted := make(chan struct{}, 1)
	releaseQuery := make(chan struct{})
	interests := make(chan string, 64)
	var queries atomic.Int32
	r := &ServiceResolver{
		translationTable: TableEntryCache.NewTableManager(),
		tableQuery: func(got netip.Addr) ([]TableEntryCache.TableEntry, error) {
			if got != addr {
				t.Errorf("queried %s; want %s", got, addr)
			}
			queries.Add(1)
			select {
			case queryStarted <- struct{}{}:
			default:
			}
			<-releaseQuery
			return []TableEntryCache.TableEntry{firstEntry, secondEntry}, nil
		},
		interestRegistrar: func(target string) {
			interests <- target
		},
	}

	first := r.GetTableEntryByServiceIP(addr)
	if len(first.Entries) != 0 || first.Resolving == nil {
		t.Fatalf("cold lookup = %+v; want no entries and an in-flight resolution", first)
	}
	select {
	case <-queryStarted:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the table query to start")
	}

	// A burst behind the same cold VIP must join the existing query rather than
	// starting one MQTT round trip per packet.
	for i := 0; i < 16; i++ {
		lookup := r.GetTableEntryByServiceIP(addr)
		if lookup.Resolving != first.Resolving {
			t.Fatalf("lookup %d received a different resolution channel", i)
		}
	}
	if got := queries.Load(); got != 1 {
		t.Fatalf("started %d table queries for one Service IP; want 1", got)
	}

	close(releaseQuery)
	select {
	case <-first.Resolving:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the successful resolution")
	}

	activity := events.GetOrCreate(job)
	if idle := activity.IdleFor(); idle != time.Duration(1<<63-1) {
		t.Fatalf("activity was touched before the first cache hit: idle for %v", idle)
	}
	lookup := r.GetTableEntryByServiceIP(addr)
	if lookup.Resolving != nil {
		t.Error("a resolved lookup still reports an in-flight query")
	}
	if len(lookup.Entries) != 2 {
		t.Fatalf("installed %d entries; want the complete 2-entry response", len(lookup.Entries))
	}
	if lookup.Generation == 0 {
		t.Error("resolved entries were returned without a table generation")
	}
	if got := queries.Load(); got != 1 {
		t.Errorf("cache hit started another table query; total queries = %d", got)
	}
	if activity.IdleFor() == time.Duration(1<<63-1) {
		t.Error("the cache hit did not touch the job activity tracker")
	}

	for i, want := range []string{job, vip} {
		select {
		case got := <-interests:
			if got != want {
				t.Errorf("interest %d registered for %q; want %q", i, got, want)
			}
		default:
			t.Fatalf("interest %d for %q was not registered", i, want)
		}
	}
	select {
	case extra := <-interests:
		t.Errorf("unexpected extra interest registered for %q", extra)
	default:
	}
}

// a rejected response must not displace the existing route sharing its namespace IP
func TestWrongAddressResponseLeavesTableUntouched(t *testing.T) {
	const wantedVIP = "10.30.9.9"

	good := resolvableEntry("goodjob", "10.30.1.1")

	r := &ServiceResolver{
		translationTable: TableEntryCache.NewTableManager(),
		tableQuery: func(netip.Addr) ([]TableEntryCache.TableEntry, error) {
			// valid response, but for a different job and a different Service IP than asked
			return []TableEntryCache.TableEntry{resolvableEntry("otherjob", "10.30.2.2")}, nil
		},
	}
	if err := r.translationTable.Add(good); err != nil {
		t.Fatal(err)
	}

	if err := r.resolveServiceIP(netip.MustParseAddr(wantedVIP)); err == nil {
		t.Fatal("expected an error for a response that does not resolve the requested address")
	}

	if got := len(r.translationTable.SearchByJobName("goodjob")); got != 1 {
		t.Errorf("the rejected response displaced the existing route: goodjob has %d entries, want 1", got)
	}
	if got := len(r.translationTable.SearchByJobName("otherjob")); got != 0 {
		t.Errorf("the rejected response was installed anyway: otherjob has %d entries, want 0", got)
	}
	if entries, _ := r.translationTable.SearchByServiceIP(netip.MustParseAddr("10.30.1.1")); len(entries) != 1 {
		t.Errorf("the existing route is no longer resolvable: %d entries", len(entries))
	}
}

func TestMixedJobResponseRejected(t *testing.T) {
	r := &ServiceResolver{
		translationTable: TableEntryCache.NewTableManager(),
		tableQuery: func(netip.Addr) ([]TableEntryCache.TableEntry, error) {
			first := resolvableEntry("jobone", "10.30.9.9")
			second := resolvableEntry("jobtwo", "10.30.9.9")
			second.Nsip = net.ParseIP("10.19.1.2")
			return []TableEntryCache.TableEntry{first, second}, nil
		},
	}

	if err := r.resolveServiceIP(netip.MustParseAddr("10.30.9.9")); err == nil {
		t.Fatal("expected an error for a response mixing job names")
	}
	if got := len(r.translationTable.SearchByJobName("jobone")); got != 0 {
		t.Errorf("a malformed response was partially installed: jobone has %d entries", got)
	}
}

func queryInstance(number int, vip string) mqttifce.ServiceInstance {
	return mqttifce.ServiceInstance{
		InstanceNumber: number,
		NamespaceIp:    fmt.Sprintf("10.19.1.%d", number+1),
		NamespaceIpv6:  fmt.Sprintf("fc00::%x", number+1),
		HostIp:         "10.0.0.1",
		HostPort:       50103,
		ServiceIp:      []mqttifce.Sip{{Type: "RR", Address: vip, Address_v6: "fdff::1"}},
	}
}

func queryResponse(instances ...mqttifce.ServiceInstance) mqttifce.TableQueryResponse {
	return mqttifce.TableQueryResponse{JobName: "app.ns.svc.svcns", InstanceList: instances}
}

func parsedQuery(response mqttifce.TableQueryResponse) func(netip.Addr) ([]TableEntryCache.TableEntry, error) {
	return func(netip.Addr) ([]TableEntryCache.TableEntry, error) {
		return responseParser(response)
	}
}

// one bad instance must not cost the job its healthy ones
func TestInvalidInstanceDoesNotBlockTheRest(t *testing.T) {
	const vip = "10.30.9.9"
	bad := queryInstance(0, vip)
	bad.HostIp = ""

	r := &ServiceResolver{
		translationTable:  TableEntryCache.NewTableManager(),
		interestRegistrar: func(string) {},
		tableQuery:        parsedQuery(queryResponse(bad, queryInstance(1, vip))),
	}

	if err := r.resolveServiceIP(netip.MustParseAddr(vip)); err != nil {
		t.Fatalf("a response with a valid instance was rejected: %v", err)
	}
	entries := r.translationTable.SearchByJobName("app.ns.svc.svcns")
	if len(entries) != 1 || entries[0].Instancenumber != 1 {
		t.Errorf("want only the valid instance installed, got %+v", entries)
	}
}

// skipping the invalid entry must not let the query pass as a success: addr
// would stay unresolved with the negative cache unarmed
func TestAddressOnlyOnInvalidInstanceIsAFailure(t *testing.T) {
	const wantedVIP = "10.30.9.9"
	bad := queryInstance(1, wantedVIP)
	bad.HostIp = ""

	r := &ServiceResolver{
		translationTable:  TableEntryCache.NewTableManager(),
		interestRegistrar: func(string) {},
		tableQuery:        parsedQuery(queryResponse(queryInstance(0, "10.30.1.1"), bad)),
	}

	if err := r.resolveServiceIP(netip.MustParseAddr(wantedVIP)); err == nil {
		t.Fatal("expected an error when only an invalid instance advertises the address")
	}
	if got := len(r.translationTable.SearchByJobName("app.ns.svc.svcns")); got != 0 {
		t.Errorf("the failed query installed %d entries", got)
	}
}

func TestResponseWithNoValidInstanceIsAnError(t *testing.T) {
	bad := queryInstance(0, "10.30.9.9")
	bad.HostIp = ""
	if _, err := responseParser(queryResponse(bad)); err == nil {
		t.Fatal("expected an error, an empty result would wipe the job's routes")
	}
}

func TestEmptyResponseMeansNoInstances(t *testing.T) {
	entries, err := responseParser(queryResponse())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("got %d entries from an empty response", len(entries))
	}
}
