// Package tui renders the lantern terminal UI with Bubble Tea.
package tui

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Meru143/lantern/internal/bus"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type tabModel interface {
	Title() string
	Refresh() error
	Resize(width, height int)
	Update(msg tea.Msg) tea.Cmd
	View() string
}

type busEventMsg struct {
	event bus.Event
}

type model struct {
	activeTab    int
	tabs         []tabModel
	width        int
	height       int
	eventBus     *bus.EventBus
	subscriberID string
	subscription <-chan bus.Event
	errMessage   string
}

var (
	tabStyle = lipgloss.NewStyle().
			Padding(0, 2).
			Foreground(lipgloss.Color("#9BA9B7"))
	activeTabStyle = tabStyle.
			Foreground(lipgloss.Color("#111417")).
			Background(lipgloss.Color("#FFD166")).
			Bold(true)
	rootStyle = lipgloss.NewStyle().
			Padding(1, 2)
	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EF476F")).
			Bold(true)
)

// NewModel creates the root Bubble Tea model for lantern's TUI.
func NewModel(db *sql.DB, eventBus *bus.EventBus) model {
	subscriberID := fmt.Sprintf("tui-%d", time.Now().UnixNano())
	var subscription <-chan bus.Event
	if eventBus != nil {
		subscription = eventBus.Subscribe(subscriberID, 64)
	}

	tabs := []tabModel{
		newTracesTab(db),
		newLogsTab(db),
		newServicesTab(db),
		newErrorsTab(db),
	}

	m := model{
		tabs:         tabs,
		eventBus:     eventBus,
		subscriberID: subscriberID,
		subscription: subscription,
	}
	m.refreshAll()

	return m
}

// Init starts listening for live bus events.
func (m model) Init() tea.Cmd {
	return waitForBusEvent(m.subscription)
}

// Update handles root-level TUI messages.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = typed.Width
		m.height = typed.Height
		m.resizeTabs()
		return m, nil
	case tea.KeyMsg:
		switch typed.String() {
		case "ctrl+c", "q":
			m.cleanup()
			return m, tea.Quit
		case "tab", "right", "l":
			m.activeTab = (m.activeTab + 1) % len(m.tabs)
			return m, nil
		case "shift+tab", "left", "h":
			m.activeTab = (m.activeTab - 1 + len(m.tabs)) % len(m.tabs)
			return m, nil
		}
	case busEventMsg:
		m.refreshAll()
		return m, waitForBusEvent(m.subscription)
	}

	if len(m.tabs) == 0 {
		return m, nil
	}

	cmd := m.tabs[m.activeTab].Update(msg)
	return m, cmd
}

// View renders the root TUI.
func (m model) View() string {
	if len(m.tabs) == 0 {
		return rootStyle.Render("No tabs configured.")
	}

	tabViews := make([]string, 0, len(m.tabs))
	for index, tab := range m.tabs {
		style := tabStyle
		if index == m.activeTab {
			style = activeTabStyle
		}
		tabViews = append(tabViews, style.Render(tab.Title()))
	}

	body := m.tabs[m.activeTab].View()
	if m.errMessage != "" {
		body = errorStyle.Render(m.errMessage) + "\n\n" + body
	}

	return rootStyle.Render(lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.JoinHorizontal(lipgloss.Top, tabViews...),
		"",
		body,
	))
}

func (m *model) resizeTabs() {
	contentWidth := m.width - 4
	if contentWidth < 20 {
		contentWidth = 20
	}
	contentHeight := m.height - 5
	if contentHeight < 8 {
		contentHeight = 8
	}

	for _, tab := range m.tabs {
		tab.Resize(contentWidth, contentHeight)
	}
}

func (m *model) refreshAll() {
	m.errMessage = ""
	for _, tab := range m.tabs {
		if err := tab.Refresh(); err != nil {
			m.errMessage = err.Error()
		}
	}
	m.resizeTabs()
}

func (m *model) cleanup() {
	if m.eventBus != nil && m.subscriberID != "" {
		m.eventBus.Unsubscribe(m.subscriberID)
		m.subscriberID = ""
	}
}

func waitForBusEvent(subscription <-chan bus.Event) tea.Cmd {
	if subscription == nil {
		return nil
	}

	return func() tea.Msg {
		event, ok := <-subscription
		if !ok {
			return nil
		}
		return busEventMsg{event: event}
	}
}

func contextBackground() context.Context {
	return context.Background()
}

func shortDuration(nanos int64) string {
	switch {
	case nanos >= 1_000_000_000:
		return fmt.Sprintf("%.2fs", float64(nanos)/1_000_000_000)
	case nanos >= 1_000_000:
		return fmt.Sprintf("%.1fms", float64(nanos)/1_000_000)
	default:
		return fmt.Sprintf("%dus", nanos/1_000)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func boolLabel(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func padRight(value string, width int) string {
	for len(value) < width {
		value += " "
	}
	return value
}

func max(left, right int) int {
	if left > right {
		return left
	}
	return right
}
