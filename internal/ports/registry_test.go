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
	base, max := freeRange(t, 10)
	requests := []PortRequest{{Service: "web", ContainerPort: 80, HostIP: "127.0.0.1"}}
	first, err := reg.Allocate("one", requests, Range{Min: base, Max: max})
	if err != nil {
		t.Fatal(err)
	}
	second, err := reg.Allocate("one", requests, Range{Min: base, Max: max})
	if err != nil {
		t.Fatal(err)
	}
	if first[0].HostPort != second[0].HostPort {
		t.Fatalf("port not reused: %#v vs %#v", first, second)
	}
	other, err := reg.Allocate("two", requests, Range{Min: base, Max: max})
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
	base, max := freeRange(t, 10)
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(base)))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	reg := &Registry{Dir: t.TempDir()}
	requests := []PortRequest{{Service: "web", ContainerPort: 80, HostIP: "127.0.0.1"}}
	got, err := reg.Allocate("one", requests, Range{Min: base, Max: max})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].HostPort == base {
		t.Fatalf("allocated already-bound port %d", base)
	}
}

func TestAllocateReplacesTakenExistingPort(t *testing.T) {
	base, max := freeRange(t, 10)
	reg := &Registry{Dir: t.TempDir()}
	requests := []PortRequest{{Service: "web", ContainerPort: 80, HostIP: "127.0.0.1"}}
	first, err := reg.Allocate("one", requests, Range{Min: base, Max: max})
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
	base, max := freeRange(t, 10)
	reg := &Registry{Dir: t.TempDir()}
	requests := []PortRequest{{Service: "web", ContainerPort: 80, HostIP: "127.0.0.1"}}
	first, err := reg.Allocate("one", requests, Range{Min: base, Max: max})
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(first[0].HostPort)))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	existing, ok, err := reg.ExistingAssignments("one", requests)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ExistingAssignments returned false for own instance")
	}
	if existing[0].HostPort != first[0].HostPort {
		t.Fatalf("expected to keep own bound port %d, got %d", first[0].HostPort, existing[0].HostPort)
	}
}

// freeRange finds a contiguous block of `count` TCP ports on 127.0.0.1
// that are all available right now by listening on each in sequence.
func freeRange(t *testing.T, count int) (int, int) {
	t.Helper()
	for attempt := 0; attempt < 50; attempt++ {
		baseL, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		base := baseL.Addr().(*net.TCPAddr).Port
		baseL.Close()

		listeners := make([]net.Listener, 0, count)
		ok := true
		for i := 0; i < count; i++ {
			l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(base+i)))
			if err != nil {
				ok = false
				break
			}
			listeners = append(listeners, l)
		}
		if ok {
			for _, l := range listeners {
				l.Close()
			}
			return base, base + count - 1
		}
		for _, l := range listeners {
			l.Close()
		}
	}
	t.Fatal("could not find a free contiguous port range after 50 attempts")
	return 0, 0
}
