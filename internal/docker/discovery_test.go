package docker

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestFindHostVethByIfindex(t *testing.T) {
	// Find the loopback interface ifindex as a known reference.
	loIdxBytes, err := os.ReadFile("/sys/class/net/lo/ifindex")
	if err != nil {
		t.Skip("cannot read loopback ifindex:", err)
	}
	loIdx, err := strconv.Atoi(strings.TrimSpace(string(loIdxBytes)))
	if err != nil {
		t.Fatalf("parse lo ifindex: %v", err)
	}

	if got := findHostVethByIfindex(loIdx); got != "" {
		t.Fatalf("lo is not a veth, expected empty, got %q", got)
	}
}

func TestFirstHostVeth(t *testing.T) {
	// Just ensure it doesn't panic and returns either empty or a veth-prefixed name.
	name := firstHostVeth()
	if name != "" && !isVeth(name) {
		t.Fatalf("expected veth name, got %q", name)
	}
}

func isVeth(name string) bool {
	return len(name) >= 4 && name[:4] == "veth"
}
