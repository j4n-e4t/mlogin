package main

import (
	"errors"
	"testing"
)

func TestIsIgnorableBootoutError(t *testing.T) {
	cases := []struct {
		err       error
		ignorable bool
	}{
		{err: errors.New("Boot-out failed: 125: Domain does not support specified action"), ignorable: true},
		{err: errors.New("bootout: No such process"), ignorable: true},
		{err: errors.New("Service could not be found"), ignorable: true},
		{err: errors.New("permission denied"), ignorable: false},
	}

	for _, tc := range cases {
		got := isIgnorableBootoutError(tc.err)
		if got != tc.ignorable {
			t.Fatalf("isIgnorableBootoutError(%q) = %v, want %v", tc.err.Error(), got, tc.ignorable)
		}
	}
}

func TestParseBundleVersion(t *testing.T) {
	bundle, version := parseBundleVersion("io.tailscale.ipn.macsys.network-extension (1.94.1/101.94.1)")
	if bundle != "io.tailscale.ipn.macsys.network-extension" {
		t.Fatalf("unexpected bundle: %q", bundle)
	}
	if version != "1.94.1/101.94.1" {
		t.Fatalf("unexpected version: %q", version)
	}
}

func TestSplitTabColumns(t *testing.T) {
	line := "*\t*\tW5364U7YZB\tio.tailscale.ipn.macsys.network-extension (1.94.1/101.94.1)\tTailscale Network Extension\t[activated enabled]"
	cols := splitTabColumns(line)
	if len(cols) != 6 {
		t.Fatalf("expected 6 cols, got %d (%v)", len(cols), cols)
	}
	if cols[2] != "W5364U7YZB" {
		t.Fatalf("unexpected team id column: %q", cols[2])
	}
}
