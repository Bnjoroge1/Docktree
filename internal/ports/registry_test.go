package ports

import (
	"net"
	"strconv"
	"testing"
)

func TestParseRange(t *testing.T) {
	r, err := ParseRange("41000-49999")
	if err != nil {
		t.Fatal(err)
	}
	if r.Min != 41000 || r.Max != 49999 {
		t.Fatalf("bad range: %#v", r)
	}
}

func TestAllocateReuseAndRelease(t *testing.T) {
	reg := &Registry{Dir: t.TempDir()}
	if err := reg.Lock(); err != nil {
		t.Fatal(err)
	}
	defer reg.Unlock()
	requests := []PortRequest{{Service: "web", ContainerPort: 80, HostIP: "127.0.0.1"}}
	first, err := reg.Allocate("one", requests, Range{Min: 41000, Max: 41010})
	if err != nil {
		t.Fatal(err)
	}
	second, err := reg.Allocate("one", requests, Range{Min: 41000, Max: 41010})
	if err != nil {
		t.Fatal(err)
	}
	if first[0].HostPort != second[0].HostPort {
		t.Fatalf("port not reused: %#v vs %#v", first, second)
	}
	other, err := reg.Allocate("two", requests, Range{Min: 41000, Max: 41010})
	if err != nil {
		t.Fatal(err)
	}
	if first[0].HostPort == other[0].HostPort {
		t.Fatalf("two instances got same port: %d", first[0].HostPort)
	}
	if err := reg.Release("one"); err != nil {
		t.Fatal(err)
	}
	all, err := reg.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := all["one"]; ok {
		t.Fatalf("instance was not released: %#v", all)
	}
}

func TestAllocateSkipsBoundPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	reg := &Registry{Dir: t.TempDir()}
	got, err := reg.Allocate("one", []PortRequest{{Service: "web", ContainerPort: 80, HostIP: "127.0.0.1"}}, Range{Min: port, Max: port + 1})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].HostPort == port {
		t.Fatalf("allocated already-bound port %d", port)
	}
}

func TestAllocateReplacesTakenExistingPort(t *testing.T) {
	reg := &Registry{Dir: t.TempDir()}
	requests := []PortRequest{{Service: "web", ContainerPort: 80, HostIP: "127.0.0.1"}}
	first, err := reg.Allocate("one", requests, Range{Min: 41000, Max: 41010})
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(first[0].HostPort)))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	second, err := reg.Allocate("one", requests, Range{Min: first[0].HostPort, Max: first[0].HostPort + 1})
	if err != nil {
		t.Fatal(err)
	}
	if second[0].HostPort == first[0].HostPort {
		t.Fatalf("reused taken port %d", first[0].HostPort)
	}
}

func TestExistingAssignmentsKeepsCurrentlyBoundOwnPort(t *testing.T) {
	reg := &Registry{Dir: t.TempDir()}
	requests := []PortRequest{{Service: "web", ContainerPort: 80, HostIP: "127.0.0.1"}}
	first, err := reg.Allocate("one", requests, Range{Min: 41000, Max: 41010})
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(first[0].HostPort)))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	kept, ok, err := reg.ExistingAssignments("one", requests)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected existing assignments to cover requested ports")
	}
	if kept[0].HostPort != first[0].HostPort {
		t.Fatalf("expected to keep existing port %d, got %d", first[0].HostPort, kept[0].HostPort)
	}
}
