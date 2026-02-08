package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

type LoginItem struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Hidden bool   `json:"hidden"`
}

type BackgroundItem struct {
	Label    string `json:"label"`
	Path     string `json:"path"`
	Scope    string `json:"scope"`
	Kind     string `json:"kind"`
	Loaded   bool   `json:"loaded"`
	Disabled *bool  `json:"disabled,omitempty"`
}

type SystemExtensionItem struct {
	Category string `json:"category"`
	Enabled  bool   `json:"enabled"`
	Active   bool   `json:"active"`
	TeamID   string `json:"team_id"`
	BundleID string `json:"bundle_id"`
	Version  string `json:"version,omitempty"`
	Name     string `json:"name"`
	State    string `json:"state"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return nil
	}

	switch args[0] {
	case "version", "--version", "-v":
		printVersion()
		return nil
	case "login":
		return runLogin(args[1:])
	case "background", "bg":
		return runBackground(args[1:])
	case "extensions", "ext":
		return runExtensions(args[1:])
	case "tui", "ui":
		return runTUI()
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func printUsage() {
	fmt.Println(`mlogin - manage macOS login and background items

Usage:
  mlogin version
  mlogin tui

  mlogin login list [--json]
  mlogin login add --path <app path> [--hidden]
  mlogin login remove (--name <item name> | --path <app path>)

  mlogin background list [--json] [--scope user|system|all]
  mlogin background enable --label <label> [--scope user|system]
  mlogin background disable --label <label> [--scope user|system]
  mlogin background load --plist <plist path> [--scope user|system]
  mlogin background unload --label <label> [--scope user|system]
  mlogin background delete --label <label> --plist <plist path> [--scope user|system]
  mlogin extensions list [--json]

Notes:
  - tui gives an interactive table view and quick actions.
  - login commands use System Events via osascript.
  - system background commands may require sudo.`)
}

func printVersion() {
	fmt.Printf("mlogin %s\n", version)
	fmt.Printf("commit: %s\n", commit)
	fmt.Printf("built: %s\n", date)
}

func runLogin(args []string) error {
	if len(args) == 0 {
		return errors.New("missing login subcommand")
	}

	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("login list", flag.ContinueOnError)
		jsonOut := fs.Bool("json", false, "output JSON")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		items, err := listLoginItems()
		if err != nil {
			return err
		}
		if *jsonOut {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(items)
		}
		printLoginItems(items)
		return nil
	case "add":
		fs := flag.NewFlagSet("login add", flag.ContinueOnError)
		path := fs.String("path", "", "app path")
		hidden := fs.Bool("hidden", false, "start hidden")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *path == "" {
			return errors.New("--path is required")
		}
		return addLoginItem(*path, *hidden)
	case "remove":
		fs := flag.NewFlagSet("login remove", flag.ContinueOnError)
		name := fs.String("name", "", "login item name")
		path := fs.String("path", "", "login item app path")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *name == "" && *path == "" {
			return errors.New("provide --name or --path")
		}
		return removeLoginItem(*name, *path)
	default:
		return fmt.Errorf("unknown login subcommand %q", args[0])
	}
}

func runBackground(args []string) error {
	if len(args) == 0 {
		return errors.New("missing background subcommand")
	}

	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("background list", flag.ContinueOnError)
		jsonOut := fs.Bool("json", false, "output JSON")
		scope := fs.String("scope", "all", "user|system|all")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		items, warnings, err := listBackgroundItems(*scope)
		if err != nil {
			return err
		}
		for _, w := range warnings {
			fmt.Fprintln(os.Stderr, "warning:", w)
		}
		if *jsonOut {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(items)
		}
		printBackgroundItems(items)
		return nil
	case "enable", "disable":
		fs := flag.NewFlagSet("background enable/disable", flag.ContinueOnError)
		label := fs.String("label", "", "launchd label")
		scope := fs.String("scope", "user", "user|system")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *label == "" {
			return errors.New("--label is required")
		}
		domain, err := launchDomain(*scope)
		if err != nil {
			return err
		}
		verb := args[0]
		if err := runLaunchctl(verb, domain+"/"+*label); err != nil {
			return err
		}
		fmt.Printf("%sd %s in %s\n", verb, *label, domain)
		return nil
	case "load":
		fs := flag.NewFlagSet("background load", flag.ContinueOnError)
		plist := fs.String("plist", "", "plist path")
		scope := fs.String("scope", "user", "user|system")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *plist == "" {
			return errors.New("--plist is required")
		}
		domain, err := launchDomain(*scope)
		if err != nil {
			return err
		}
		if err := runLaunchctl("bootstrap", domain, *plist); err != nil {
			return err
		}
		fmt.Printf("loaded %s into %s\n", *plist, domain)
		return nil
	case "unload":
		fs := flag.NewFlagSet("background unload", flag.ContinueOnError)
		label := fs.String("label", "", "launchd label")
		scope := fs.String("scope", "user", "user|system")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *label == "" {
			return errors.New("--label is required")
		}
		domain, err := launchDomain(*scope)
		if err != nil {
			return err
		}
		if err := runLaunchctl("bootout", domain+"/"+*label); err != nil {
			return err
		}
		fmt.Printf("unloaded %s from %s\n", *label, domain)
		return nil
	case "delete", "remove":
		fs := flag.NewFlagSet("background delete", flag.ContinueOnError)
		label := fs.String("label", "", "launchd label")
		plist := fs.String("plist", "", "plist path")
		scope := fs.String("scope", "user", "user|system")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *label == "" || *plist == "" {
			return errors.New("--label and --plist are required")
		}
		return deleteBackgroundItem(*label, *plist, *scope)
	default:
		return fmt.Errorf("unknown background subcommand %q", args[0])
	}
}

func runExtensions(args []string) error {
	if len(args) == 0 {
		return errors.New("missing extensions subcommand")
	}
	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("extensions list", flag.ContinueOnError)
		jsonOut := fs.Bool("json", false, "output JSON")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		items, err := listSystemExtensions()
		if err != nil {
			return err
		}
		if *jsonOut {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(items)
		}
		printSystemExtensions(items)
		return nil
	default:
		return fmt.Errorf("unknown extensions subcommand %q", args[0])
	}
}

func deleteBackgroundItem(label, plistPath, scope string) error {
	absPath, err := filepath.Abs(plistPath)
	if err != nil {
		return err
	}
	domain, err := launchDomain(scope)
	if err != nil {
		return err
	}

	// Attempt to stop the service first; if already stopped or not found, continue.
	if err := runLaunchctl("bootout", domain+"/"+label); err != nil {
		if !isIgnorableBootoutError(err) {
			return fmt.Errorf("bootout failed for %s: %w", label, err)
		}
	}

	if err := os.Remove(absPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("remove plist %s: %w", absPath, err)
	}

	fmt.Printf("deleted background item %s (%s)\n", label, absPath)
	return nil
}

func isIgnorableBootoutError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such process") ||
		strings.Contains(msg, "service could not be found") ||
		strings.Contains(msg, "not found") ||
		strings.Contains(msg, "domain does not support specified action")
}

func listLoginItems() ([]LoginItem, error) {
	script := `
ObjC.import('Cocoa');
const se = Application('System Events');
const items = se.loginItems();
const out = items.map((item) => {
  return {
    name: item.name(),
    path: item.path(),
    hidden: item.hidden()
  };
});
	$.NSFileHandle.fileHandleWithStandardOutput.writeData($(JSON.stringify(out) + "\n").dataUsingEncoding($.NSUTF8StringEncoding));
`
	stdout, stderr, err := runOSA(script, nil)
	if err != nil {
		return nil, fmt.Errorf("osascript login list failed: %w: %s", err, strings.TrimSpace(stderr))
	}
	var items []LoginItem
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &items); err != nil {
		return nil, fmt.Errorf("parse login items: %w", err)
	}
	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	return items, nil
}

func addLoginItem(path string, hidden bool) error {
	abspath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	hiddenJS := "false"
	if hidden {
		hiddenJS = "true"
	}
	script := fmt.Sprintf(`
const se = Application('System Events');
const existing = se.loginItems.whose({path: %q})();
for (const item of existing) {
  item.delete();
}
se.loginItems.push(se.LoginItem({path: %q, hidden: %s}));
`, abspath, abspath, hiddenJS)
	_, stderr, err := runOSA(script, nil)
	if err != nil {
		return fmt.Errorf("add login item failed: %w: %s", err, strings.TrimSpace(stderr))
	}
	fmt.Printf("added login item: %s\n", abspath)
	return nil
}

func removeLoginItem(name, path string) error {
	script := `
const se = Application('System Events');
let removed = 0;
const all = se.loginItems();
for (const item of all) {
  const matchesName = $.getenv('REMOVE_NAME') ? item.name() === $.getenv('REMOVE_NAME') : false;
  const matchesPath = $.getenv('REMOVE_PATH') ? item.path() === $.getenv('REMOVE_PATH') : false;
  if (matchesName || matchesPath) {
    item.delete();
    removed += 1;
  }
}
if (removed === 0) {
  throw new Error('no matching login item found');
}
`
	env := map[string]string{}
	if name != "" {
		env["REMOVE_NAME"] = name
	}
	if path != "" {
		abspath, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		env["REMOVE_PATH"] = abspath
	}
	_, stderr, err := runOSA(script, env)
	if err != nil {
		return fmt.Errorf("remove login item failed: %w: %s", err, strings.TrimSpace(stderr))
	}
	fmt.Println("removed matching login items")
	return nil
}

func listBackgroundItems(scope string) ([]BackgroundItem, []string, error) {
	scope = strings.ToLower(scope)
	if scope != "user" && scope != "system" && scope != "all" {
		return nil, nil, errors.New("scope must be user, system, or all")
	}

	var dirs []struct {
		scope string
		kind  string
		dir   string
	}
	if scope == "user" || scope == "all" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, nil, err
		}
		dirs = append(dirs, struct {
			scope string
			kind  string
			dir   string
		}{scope: "user", kind: "agent", dir: filepath.Join(home, "Library/LaunchAgents")})
	}
	if scope == "system" || scope == "all" {
		dirs = append(dirs,
			struct {
				scope string
				kind  string
				dir   string
			}{scope: "system", kind: "agent", dir: "/Library/LaunchAgents"},
			struct {
				scope string
				kind  string
				dir   string
			}{scope: "system", kind: "daemon", dir: "/Library/LaunchDaemons"},
		)
	}

	loadedUser := map[string]bool{}
	if scope == "user" || scope == "all" {
		labels, err := getLoadedUserLabels()
		if err == nil {
			loadedUser = labels
		}
	}

	disabledByScope := map[string]map[string]bool{}
	warnings := []string{}
	if scope == "user" || scope == "all" {
		domain, err := launchDomain("user")
		if err == nil {
			m, err := getDisabledLabels(domain)
			if err != nil {
				warnings = append(warnings, "could not read user disabled state: "+err.Error())
			} else {
				disabledByScope["user"] = m
			}
		}
	}
	if scope == "system" || scope == "all" {
		m, err := getDisabledLabels("system")
		if err != nil {
			warnings = append(warnings, "could not read system disabled state (try sudo): "+err.Error())
		} else {
			disabledByScope["system"] = m
		}
	}

	var items []BackgroundItem
	for _, d := range dirs {
		entries, err := os.ReadDir(d.dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			warnings = append(warnings, fmt.Sprintf("could not read %s: %v", d.dir, err))
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".plist") {
				continue
			}
			p := filepath.Join(d.dir, e.Name())
			label, err := readPlistLabel(p)
			if err != nil || label == "" {
				continue
			}
			item := BackgroundItem{
				Label:  label,
				Path:   p,
				Scope:  d.scope,
				Kind:   d.kind,
				Loaded: d.scope == "user" && loadedUser[label],
			}
			if m, ok := disabledByScope[d.scope]; ok {
				if disabled, exists := m[label]; exists {
					v := disabled
					item.Disabled = &v
				}
			}
			items = append(items, item)
		}
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Scope != items[j].Scope {
			return items[i].Scope < items[j].Scope
		}
		return items[i].Label < items[j].Label
	})
	return items, warnings, nil
}

func listSystemExtensions() ([]SystemExtensionItem, error) {
	cmd := exec.Command("systemextensionsctl", "list")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var items []SystemExtensionItem
	currentCategory := ""
	s := bufio.NewScanner(bytes.NewReader(out))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasSuffix(line, "extension(s)") {
			continue
		}
		if strings.HasPrefix(line, "--- ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				currentCategory = parts[1]
			}
			continue
		}
		if strings.HasPrefix(line, "enabled") {
			continue
		}

		cols := splitTabColumns(line)
		if len(cols) < 6 {
			continue
		}
		bundleID, version := parseBundleVersion(cols[3])
		state := strings.Trim(cols[5], "[]")
		items = append(items, SystemExtensionItem{
			Category: currentCategory,
			Enabled:  cols[0] == "*",
			Active:   cols[1] == "*",
			TeamID:   cols[2],
			BundleID: bundleID,
			Version:  version,
			Name:     cols[4],
			State:    state,
		})
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Category != items[j].Category {
			return items[i].Category < items[j].Category
		}
		return items[i].Name < items[j].Name
	})
	return items, nil
}

func splitTabColumns(line string) []string {
	raw := strings.Split(line, "\t")
	out := make([]string, 0, len(raw))
	for _, p := range raw {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func parseBundleVersion(value string) (string, string) {
	i := strings.LastIndex(value, " (")
	if i == -1 || !strings.HasSuffix(value, ")") {
		return value, ""
	}
	return value[:i], strings.TrimSuffix(strings.TrimPrefix(value[i+1:], "("), ")")
}

func readPlistLabel(path string) (string, error) {
	cmd := exec.Command("/usr/libexec/PlistBuddy", "-c", "Print :Label", path)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func getLoadedUserLabels() (map[string]bool, error) {
	cmd := exec.Command("launchctl", "list")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	labels := map[string]bool{}
	s := bufio.NewScanner(bytes.NewReader(out))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "PID") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}
		labels[parts[2]] = true
	}
	return labels, s.Err()
}

func getDisabledLabels(domain string) (map[string]bool, error) {
	cmd := exec.Command("launchctl", "print-disabled", domain)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	labels := map[string]bool{}
	s := bufio.NewScanner(bytes.NewReader(out))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if !strings.Contains(line, "=>") {
			continue
		}
		parts := strings.Split(line, "=>")
		if len(parts) != 2 {
			continue
		}
		label := strings.Trim(strings.TrimSpace(parts[0]), `"`)
		state := strings.Trim(strings.TrimSpace(strings.TrimSuffix(parts[1], ";")), `"`)
		if label == "" {
			continue
		}
		labels[label] = state == "disabled" || state == "true"
	}
	return labels, s.Err()
}

func runLaunchctl(args ...string) error {
	cmd := exec.Command("launchctl", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
		return err
	}
	return nil
}

func launchDomain(scope string) (string, error) {
	switch strings.ToLower(scope) {
	case "system":
		return "system", nil
	case "user":
		u, err := user.Current()
		if err != nil {
			return "", err
		}
		uid, err := strconv.Atoi(u.Uid)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("gui/%d", uid), nil
	default:
		return "", errors.New("scope must be user or system")
	}
}

func runOSA(script string, env map[string]string) (string, string, error) {
	cmd := exec.Command("osascript", "-l", "JavaScript", "-e", script)
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func printLoginItems(items []LoginItem) {
	if len(items) == 0 {
		fmt.Println("No login items found")
		return
	}
	fmt.Printf("%-32s %-6s %s\n", "NAME", "HIDDEN", "PATH")
	for _, it := range items {
		fmt.Printf("%-32s %-6t %s\n", it.Name, it.Hidden, it.Path)
	}
}

func printBackgroundItems(items []BackgroundItem) {
	if len(items) == 0 {
		fmt.Println("No background items found")
		return
	}
	fmt.Printf("%-8s %-7s %-7s %-8s %s\n", "SCOPE", "KIND", "LOADED", "DISABLE", "LABEL")
	for _, it := range items {
		disabled := "?"
		if it.Disabled != nil {
			disabled = fmt.Sprintf("%t", *it.Disabled)
		}
		fmt.Printf("%-8s %-7s %-7t %-8s %s\n", it.Scope, it.Kind, it.Loaded, disabled, it.Label)
		fmt.Printf("  %s\n", it.Path)
	}
}

func printSystemExtensions(items []SystemExtensionItem) {
	if len(items) == 0 {
		fmt.Println("No system extensions found")
		return
	}
	fmt.Printf("%-43s %-7s %-6s %-10s %-38s %s\n", "CATEGORY", "ENABLED", "ACTIVE", "TEAMID", "BUNDLEID", "NAME")
	for _, it := range items {
		fmt.Printf("%-43s %-7t %-6t %-10s %-38s %s\n", it.Category, it.Enabled, it.Active, it.TeamID, it.BundleID, it.Name)
	}
}
