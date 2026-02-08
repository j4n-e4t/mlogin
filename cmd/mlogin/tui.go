package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

type uiTab int

const (
	tabLogin uiTab = iota
	tabBackground
	tabExtensions
)

type loginLoadedMsg struct {
	items []LoginItem
	err   error
}

type backgroundLoadedMsg struct {
	items    []BackgroundItem
	warnings []string
	err      error
}

type actionDoneMsg struct {
	status string
	err    error
}

type extensionsLoadedMsg struct {
	items []SystemExtensionItem
	err   error
}

type uiModel struct {
	width int

	height int
	tab    uiTab
	table  table.Model

	loginItems []LoginItem
	loginRows  []int
	bgItems    []BackgroundItem
	bgRows     []int
	extItems   []SystemExtensionItem
	extRows    []int
	warnings   []string

	filter       string
	filterActive bool
	confirmMode  bool
	confirmText  string
	pendingBGDel *BackgroundItem
	status       string
	err          error
}

func runTUI() error {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return fmt.Errorf("tui mode requires an interactive terminal")
	}
	p := tea.NewProgram(newUIModel(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func newUIModel() uiModel {
	t := table.New(
		table.WithColumns([]table.Column{{Title: "Loading...", Width: 20}}),
		table.WithRows(nil),
		table.WithFocused(true),
		table.WithHeight(10),
	)
	styles := table.DefaultStyles()
	styles.Header = styles.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(true)
	styles.Selected = styles.Selected.
		Foreground(lipgloss.Color("230")).
		Background(lipgloss.Color("62")).
		Bold(true)
	t.SetStyles(styles)

	return uiModel{
		tab:    tabLogin,
		table:  t,
		status: "Loading login/background items...",
	}
}

func (m uiModel) Init() tea.Cmd {
	return tea.Batch(refreshLoginCmd(), refreshBackgroundCmd(), refreshExtensionsCmd())
}

func refreshLoginCmd() tea.Cmd {
	return func() tea.Msg {
		items, err := listLoginItems()
		return loginLoadedMsg{items: items, err: err}
	}
}

func refreshBackgroundCmd() tea.Cmd {
	return func() tea.Msg {
		items, warnings, err := listBackgroundItems("all")
		return backgroundLoadedMsg{items: items, warnings: warnings, err: err}
	}
}

func refreshExtensionsCmd() tea.Cmd {
	return func() tea.Msg {
		items, err := listSystemExtensions()
		return extensionsLoadedMsg{items: items, err: err}
	}
}

func removeLoginCmd(path string) tea.Cmd {
	return func() tea.Msg {
		err := removeLoginItem("", path)
		if err != nil {
			return actionDoneMsg{err: err}
		}
		return actionDoneMsg{status: "Removed login item"}
	}
}

func toggleBackgroundCmd(label, scope string, enable bool) tea.Cmd {
	return func() tea.Msg {
		domain, err := launchDomain(scope)
		if err != nil {
			return actionDoneMsg{err: err}
		}
		verb := "disable"
		if enable {
			verb = "enable"
		}
		err = runLaunchctl(verb, domain+"/"+label)
		if err != nil {
			return actionDoneMsg{err: err}
		}
		state := "disabled"
		if enable {
			state = "enabled"
		}
		return actionDoneMsg{status: fmt.Sprintf("%s %s", state, label)}
	}
}

func deleteBackgroundCmd(item BackgroundItem) tea.Cmd {
	return func() tea.Msg {
		err := deleteBackgroundItem(item.Label, item.Path, item.Scope)
		if err != nil {
			return actionDoneMsg{err: err}
		}
		return actionDoneMsg{status: fmt.Sprintf("Deleted background item %s", item.Label)}
	}
}

func (m uiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.rebuildTable(0)
		return m, nil
	case loginLoadedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.status = "Failed to load login items"
		} else {
			m.loginItems = msg.items
			m.status = fmt.Sprintf("Loaded %d login items", len(msg.items))
			m.err = nil
		}
		m.rebuildTable(0)
		return m, nil
	case backgroundLoadedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.status = "Failed to load background items"
		} else {
			m.bgItems = msg.items
			m.warnings = msg.warnings
			m.status = fmt.Sprintf("Loaded %d background items", len(msg.items))
			m.err = nil
		}
		m.rebuildTable(0)
		return m, nil
	case extensionsLoadedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.status = "Failed to load system extensions"
		} else {
			m.extItems = msg.items
			m.status = fmt.Sprintf("Loaded %d system extensions", len(msg.items))
			m.err = nil
		}
		m.rebuildTable(0)
		return m, nil
	case actionDoneMsg:
		if msg.err != nil {
			m.err = msg.err
			m.status = "Action failed"
			return m, nil
		}
		m.err = nil
		m.status = msg.status
		if m.tab == tabLogin {
			return m, refreshLoginCmd()
		}
		if m.tab == tabExtensions {
			return m, refreshExtensionsCmd()
		}
		return m, refreshBackgroundCmd()
	case tea.KeyMsg:
		if m.confirmMode {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "y":
				if m.pendingBGDel != nil {
					item := *m.pendingBGDel
					m.pendingBGDel = nil
					m.confirmMode = false
					m.confirmText = ""
					m.status = "Deleting background item..."
					return m, deleteBackgroundCmd(item)
				}
				m.pendingBGDel = nil
				m.confirmMode = false
				m.confirmText = ""
				return m, nil
			case "n", "esc":
				m.pendingBGDel = nil
				m.confirmMode = false
				m.confirmText = ""
				m.status = "Delete cancelled"
				return m, nil
			default:
				return m, nil
			}
		}

		if m.filterActive {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "enter", "esc":
				m.filterActive = false
				m.status = fmt.Sprintf("Filter applied (%d results)", len(m.table.Rows()))
				return m, nil
			case "backspace":
				if m.filter != "" {
					m.filter = trimLastRune(m.filter)
					m.rebuildTable(0)
				}
				return m, nil
			default:
				if msg.Type == tea.KeyRunes {
					m.filter += msg.String()
					m.rebuildTable(0)
				}
				return m, nil
			}
		}

		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "tab", "right", "l":
			m.tab = (m.tab + 1) % 3
			m.rebuildTable(0)
			return m, nil
		case "shift+tab", "left", "h":
			m.tab = (m.tab + 2) % 3
			m.rebuildTable(0)
			return m, nil
		case "r":
			if m.tab == tabLogin {
				m.status = "Refreshing login items..."
				return m, refreshLoginCmd()
			}
			if m.tab == tabExtensions {
				m.status = "Refreshing system extensions..."
				return m, refreshExtensionsCmd()
			}
			m.status = "Refreshing background items..."
			return m, refreshBackgroundCmd()
		case "/", "f":
			m.filterActive = true
			m.status = "Filter mode: type to filter, enter/esc to finish"
			return m, nil
		case "c":
			if m.filter != "" {
				m.filter = ""
				m.rebuildTable(0)
				m.status = "Filter cleared"
			}
			return m, nil
		case "x":
			if m.tab == tabLogin {
				item, ok := m.selectedLoginItem()
				if !ok {
					return m, nil
				}
				m.status = "Removing login item..."
				return m, removeLoginCmd(item.Path)
			}
			if m.tab == tabBackground {
				item, ok := m.selectedBackgroundItem()
				if !ok {
					return m, nil
				}
				m.pendingBGDel = &item
				m.confirmMode = true
				m.confirmText = fmt.Sprintf("Delete %s and remove plist file? (y/n)", item.Label)
				return m, nil
			}
		case "e", "d":
			if m.tab == tabBackground {
				item, ok := m.selectedBackgroundItem()
				if !ok {
					return m, nil
				}
				enable := msg.String() == "e"
				m.status = "Applying background item change..."
				return m, toggleBackgroundCmd(item.Label, item.Scope, enable)
			}
		}
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m *uiModel) selectedLoginItem() (LoginItem, bool) {
	idx := m.table.Cursor()
	if idx < 0 || idx >= len(m.loginRows) {
		return LoginItem{}, false
	}
	itemIdx := m.loginRows[idx]
	if itemIdx < 0 || itemIdx >= len(m.loginItems) {
		return LoginItem{}, false
	}
	return m.loginItems[itemIdx], true
}

func (m *uiModel) selectedBackgroundItem() (BackgroundItem, bool) {
	idx := m.table.Cursor()
	if idx < 0 || idx >= len(m.bgRows) {
		return BackgroundItem{}, false
	}
	itemIdx := m.bgRows[idx]
	if itemIdx < 0 || itemIdx >= len(m.bgItems) {
		return BackgroundItem{}, false
	}
	return m.bgItems[itemIdx], true
}

func (m *uiModel) rebuildTable(cursor int) {
	tableHeight := max(4, m.height-8)
	m.table.SetHeight(tableHeight)
	// Bubble table renders existing rows during SetColumns; clear rows first
	// so tab switches across schemas don't panic on mismatched row widths.
	m.table.SetRows(nil)

	if m.tab == tabLogin {
		nameW := max(20, m.width/5)
		hiddenW := 8
		pathW := max(30, m.width-nameW-hiddenW-8)
		m.table.SetColumns([]table.Column{
			{Title: "Name", Width: nameW},
			{Title: "Hidden", Width: hiddenW},
			{Title: "Path", Width: pathW},
		})
		rows := make([]table.Row, 0, len(m.loginItems))
		m.loginRows = nil
		for i, it := range m.loginItems {
			if !matchesLoginFilter(it, m.filter) {
				continue
			}
			rows = append(rows, table.Row{it.Name, fmt.Sprintf("%t", it.Hidden), it.Path})
			m.loginRows = append(m.loginRows, i)
		}
		m.table.SetRows(rows)
	} else {
		if m.tab == tabBackground {
			scopeW := 8
			kindW := 8
			loadedW := 8
			disabledW := 8
			labelW := max(22, m.width/4)
			pathW := max(25, m.width-scopeW-kindW-loadedW-disabledW-labelW-12)
			m.table.SetColumns([]table.Column{
				{Title: "Scope", Width: scopeW},
				{Title: "Kind", Width: kindW},
				{Title: "Loaded", Width: loadedW},
				{Title: "Disabled", Width: disabledW},
				{Title: "Label", Width: labelW},
				{Title: "Path", Width: pathW},
			})
			rows := make([]table.Row, 0, len(m.bgItems))
			m.bgRows = nil
			for i, it := range m.bgItems {
				if !matchesBackgroundFilter(it, m.filter) {
					continue
				}
				disabled := "?"
				if it.Disabled != nil {
					disabled = fmt.Sprintf("%t", *it.Disabled)
				}
				rows = append(rows, table.Row{it.Scope, it.Kind, fmt.Sprintf("%t", it.Loaded), disabled, it.Label, it.Path})
				m.bgRows = append(m.bgRows, i)
			}
			m.table.SetRows(rows)
		} else {
			catW := max(24, m.width/5)
			enabledW := 7
			activeW := 6
			teamW := 10
			bundleW := max(28, m.width/4)
			stateW := max(16, m.width/8)
			nameW := max(22, m.width-catW-enabledW-activeW-teamW-bundleW-stateW-14)
			m.table.SetColumns([]table.Column{
				{Title: "Category", Width: catW},
				{Title: "Enabled", Width: enabledW},
				{Title: "Active", Width: activeW},
				{Title: "TeamID", Width: teamW},
				{Title: "BundleID", Width: bundleW},
				{Title: "Name", Width: nameW},
				{Title: "State", Width: stateW},
			})
			rows := make([]table.Row, 0, len(m.extItems))
			m.extRows = nil
			for i, it := range m.extItems {
				if !matchesExtensionsFilter(it, m.filter) {
					continue
				}
				rows = append(rows, table.Row{
					it.Category,
					fmt.Sprintf("%t", it.Enabled),
					fmt.Sprintf("%t", it.Active),
					it.TeamID,
					it.BundleID,
					it.Name,
					it.State,
				})
				m.extRows = append(m.extRows, i)
			}
			m.table.SetRows(rows)
		}
	}

	if len(m.table.Rows()) == 0 {
		m.table.SetCursor(0)
		return
	}
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= len(m.table.Rows()) {
		cursor = len(m.table.Rows()) - 1
	}
	m.table.SetCursor(cursor)
}

func (m uiModel) View() string {
	activeTab := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62")).Padding(0, 1)
	inactiveTab := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Padding(0, 1)
	base := lipgloss.NewStyle().Foreground(lipgloss.Color("246"))
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("221"))

	loginLabel := inactiveTab.Render("Login Items")
	bgLabel := inactiveTab.Render("Background Items")
	extLabel := inactiveTab.Render("System Extensions")
	if m.tab == tabLogin {
		loginLabel = activeTab.Render("Login Items")
	} else if m.tab == tabBackground {
		bgLabel = activeTab.Render("Background Items")
	} else {
		extLabel = activeTab.Render("System Extensions")
	}

	header := lipgloss.JoinHorizontal(lipgloss.Top, loginLabel, " ", bgLabel, " ", extLabel)
	content := m.table.View()
	help := "Keys: tab switch | r refresh | / search | c clear | q quit"
	if m.tab == tabLogin {
		help = "Keys: tab switch | r refresh | / search | c clear | x delete | q quit"
	} else if m.tab == tabBackground {
		help = "Keys: tab switch | r refresh | / search | c clear | e enable | d disable | x delete | q quit"
	}
	filterLabel := "Filter: " + m.filter
	if m.filter == "" {
		filterLabel = "Filter: <none>"
	}
	if m.filterActive {
		filterLabel += " (editing)"
	}

	status := base.Render(m.status)
	if m.err != nil {
		status = errorStyle.Render("Error: " + m.err.Error())
	}
	warnings := ""
	if len(m.warnings) > 0 && m.tab == tabBackground {
		warnings = "\n" + warnStyle.Render("Warnings: "+strings.Join(m.warnings, " | "))
	}
	confirm := ""
	if m.confirmMode {
		confirm = "\n" + warnStyle.Render(m.confirmText)
	}

	return header + "\n" + base.Render(filterLabel) + "\n\n" + content + "\n\n" + base.Render(help) + "\n" + status + warnings + confirm
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func matchesLoginFilter(it LoginItem, q string) bool {
	q = strings.TrimSpace(strings.ToLower(q))
	if q == "" {
		return true
	}
	return strings.Contains(strings.ToLower(it.Name), q) || strings.Contains(strings.ToLower(it.Path), q)
}

func matchesBackgroundFilter(it BackgroundItem, q string) bool {
	q = strings.TrimSpace(strings.ToLower(q))
	if q == "" {
		return true
	}
	return strings.Contains(strings.ToLower(it.Label), q) ||
		strings.Contains(strings.ToLower(it.Path), q) ||
		strings.Contains(strings.ToLower(it.Scope), q) ||
		strings.Contains(strings.ToLower(it.Kind), q)
}

func matchesExtensionsFilter(it SystemExtensionItem, q string) bool {
	q = strings.TrimSpace(strings.ToLower(q))
	if q == "" {
		return true
	}
	return strings.Contains(strings.ToLower(it.Category), q) ||
		strings.Contains(strings.ToLower(it.TeamID), q) ||
		strings.Contains(strings.ToLower(it.BundleID), q) ||
		strings.Contains(strings.ToLower(it.Name), q) ||
		strings.Contains(strings.ToLower(it.State), q)
}

func trimLastRune(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	return string(r[:len(r)-1])
}
