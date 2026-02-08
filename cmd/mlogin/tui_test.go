package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestRebuildTableSwitchTabsDoesNotPanic(t *testing.T) {
	m := newUIModel()
	m.width = 120
	m.height = 30
	m.loginItems = []LoginItem{{Name: "Raycast", Path: "/Applications/Raycast.app", Hidden: false}}
	m.bgItems = []BackgroundItem{{
		Label:  "com.example.agent",
		Path:   "/Users/test/Library/LaunchAgents/com.example.agent.plist",
		Scope:  "user",
		Kind:   "agent",
		Loaded: true,
	}}
	m.extItems = []SystemExtensionItem{{
		Category: "com.apple.system_extension.network_extension",
		Enabled:  true,
		Active:   true,
		TeamID:   "W5364U7YZB",
		BundleID: "io.tailscale.ipn.macsys.network-extension",
		Name:     "Tailscale Network Extension",
		State:    "activated enabled",
	}}

	mustNotPanic(t, func() {
		m.tab = tabLogin
		m.rebuildTable(0)
		m.tab = tabBackground
		m.rebuildTable(0)
		m.tab = tabExtensions
		m.rebuildTable(0)
		m.tab = tabLogin
		m.rebuildTable(0)
	})
}

func TestFilterMapsRowSelectionToOriginalItems(t *testing.T) {
	m := newUIModel()
	m.width = 120
	m.height = 30
	m.loginItems = []LoginItem{
		{Name: "Alpha", Path: "/Applications/Alpha.app", Hidden: false},
		{Name: "Raycast", Path: "/Applications/Raycast.app", Hidden: false},
	}
	m.bgItems = []BackgroundItem{
		{Label: "com.foo.alpha", Path: "/tmp/a.plist", Scope: "user", Kind: "agent", Loaded: true},
		{Label: "com.foo.raycast", Path: "/tmp/r.plist", Scope: "user", Kind: "agent", Loaded: true},
	}

	m.filter = "ray"
	m.tab = tabLogin
	m.rebuildTable(0)
	if len(m.table.Rows()) != 1 {
		t.Fatalf("expected 1 login row, got %d", len(m.table.Rows()))
	}
	loginSel, ok := m.selectedLoginItem()
	if !ok || loginSel.Name != "Raycast" {
		t.Fatalf("expected Raycast selection, got ok=%v item=%+v", ok, loginSel)
	}

	m.tab = tabBackground
	m.rebuildTable(0)
	if len(m.table.Rows()) != 1 {
		t.Fatalf("expected 1 background row, got %d", len(m.table.Rows()))
	}
	bgSel, ok := m.selectedBackgroundItem()
	if !ok || bgSel.Label != "com.foo.raycast" {
		t.Fatalf("expected com.foo.raycast selection, got ok=%v item=%+v", ok, bgSel)
	}
}

func TestClearFilterKey(t *testing.T) {
	m := newUIModel()
	m.width = 120
	m.height = 30
	m.filter = "abc"
	m.loginItems = []LoginItem{{Name: "Raycast", Path: "/Applications/Raycast.app", Hidden: false}}
	m.tab = tabLogin
	m.rebuildTable(0)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	next, ok := updated.(uiModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if next.filter != "" {
		t.Fatalf("expected filter to be cleared, got %q", next.filter)
	}
}

func TestBackgroundDeleteStartsConfirmation(t *testing.T) {
	m := newUIModel()
	m.width = 120
	m.height = 30
	m.tab = tabBackground
	m.bgItems = []BackgroundItem{
		{Label: "com.foo.agent", Path: "/tmp/a.plist", Scope: "user", Kind: "agent", Loaded: true},
	}
	m.rebuildTable(0)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	next, ok := updated.(uiModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if !next.confirmMode {
		t.Fatalf("expected confirm mode to be enabled")
	}
	if next.pendingBGDel == nil || next.pendingBGDel.Label != "com.foo.agent" {
		t.Fatalf("unexpected pending delete item: %+v", next.pendingBGDel)
	}
}

func TestBackgroundDeleteCancelConfirmation(t *testing.T) {
	m := newUIModel()
	m.confirmMode = true
	m.confirmText = "Delete?"
	m.pendingBGDel = &BackgroundItem{Label: "com.foo.agent"}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	next, ok := updated.(uiModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if next.confirmMode {
		t.Fatalf("expected confirm mode to be cleared")
	}
	if next.pendingBGDel != nil {
		t.Fatalf("expected pending item to be cleared")
	}
}

func mustNotPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("unexpected panic: %v", r)
		}
	}()
	fn()
}
