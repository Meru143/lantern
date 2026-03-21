package tui

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Meru143/lantern/internal/store"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type logsTab struct {
	db       *sql.DB
	viewport viewport.Model
	width    int
	height   int
}

func newLogsTab(db *sql.DB) *logsTab {
	return &logsTab{
		db:       db,
		viewport: viewport.New(0, 0),
	}
}

func (t *logsTab) Title() string {
	return "Logs"
}

func (t *logsTab) Refresh() error {
	logs, err := store.ListLogs(contextBackground(), t.db, store.LogFilter{Limit: 200})
	if err != nil {
		return err
	}

	lines := make([]string, 0, len(logs))
	for _, record := range logs {
		timeText := lipgloss.NewStyle().Foreground(lipgloss.Color("#9BA9B7")).Render(time.Unix(0, record.Timestamp).Format("15:04:05"))
		serviceText := lipgloss.NewStyle().Foreground(lipgloss.Color("#7BDFF2")).Render(fmt.Sprintf("%-12s", firstNonEmpty(record.ServiceName, "unknown")))
		severityText := severityStyle(record).Render(fmt.Sprintf("%-5s", firstNonEmpty(record.SeverityText, fmt.Sprintf("%d", record.SeverityNum))))
		lines = append(lines, fmt.Sprintf("%s  %-12s  %-5s  %s",
			timeText,
			serviceText,
			severityText,
			record.Body,
		))
	}
	if len(lines) == 0 {
		lines = append(lines, "No logs captured yet.")
	}

	t.viewport.SetContent(strings.Join(lines, "\n"))
	return nil
}

func (t *logsTab) Resize(width, height int) {
	t.width = width
	t.height = height
	if width < 20 {
		width = 20
	}
	if height < 5 {
		height = 5
	}
	t.viewport.Width = width
	t.viewport.Height = height
}

func (t *logsTab) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	t.viewport, cmd = t.viewport.Update(msg)
	return cmd
}

func (t *logsTab) View() string {
	return t.viewport.View()
}

func severityStyle(record store.LogRecord) lipgloss.Style {
	switch {
	case strings.Contains(strings.ToUpper(record.SeverityText), "ERROR") || record.SeverityNum >= 17:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#EF476F")).Bold(true)
	case strings.Contains(strings.ToUpper(record.SeverityText), "WARN") || record.SeverityNum >= 13:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#F4A261")).Bold(true)
	case strings.Contains(strings.ToUpper(record.SeverityText), "INFO") || record.SeverityNum >= 9:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#7BDFF2")).Bold(true)
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#9BA9B7"))
	}
}
