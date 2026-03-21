package tui

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Meru143/lantern/internal/store"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type errorsTab struct {
	db       *sql.DB
	table    table.Model
	width    int
	height   int
	groups   []store.ErrorGroup
	detail   string
	expanded bool
}

func newErrorsTab(db *sql.DB) *errorsTab {
	columns := []table.Column{
		{Title: "Type", Width: 18},
		{Title: "Message", Width: 30},
		{Title: "Count", Width: 8},
		{Title: "Services", Width: 18},
	}

	model := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(10),
	)

	styles := table.DefaultStyles()
	styles.Selected = styles.Selected.
		Foreground(lipgloss.Color("#111417")).
		Background(lipgloss.Color("#7BDFF2")).
		Bold(true)
	model.SetStyles(styles)

	return &errorsTab{
		db:    db,
		table: model,
	}
}

func (t *errorsTab) Title() string {
	return "Errors"
}

func (t *errorsTab) Refresh() error {
	groups, err := store.ListErrorGroups(contextBackground(), t.db)
	if err != nil {
		return err
	}

	rows := make([]table.Row, 0, len(groups))
	for _, group := range groups {
		rows = append(rows, table.Row{
			group.ExcType,
			group.MessageNorm,
			fmt.Sprintf("%d", group.Count),
			strings.Join(group.Services, ","),
		})
	}

	t.groups = groups
	t.table.SetRows(rows)
	if cursor := t.table.Cursor(); cursor >= len(rows) && len(rows) > 0 {
		t.table.SetCursor(len(rows) - 1)
	}
	if len(rows) == 0 {
		t.expanded = false
		t.detail = ""
	}

	return nil
}

func (t *errorsTab) Resize(width, height int) {
	t.width = width
	t.height = height
	tableHeight := height - 4
	if t.expanded {
		tableHeight = height / 2
	}
	if tableHeight < 5 {
		tableHeight = 5
	}
	t.table.SetWidth(width)
	t.table.SetHeight(tableHeight)
}

func (t *errorsTab) Update(msg tea.Msg) tea.Cmd {
	switch typed := msg.(type) {
	case tea.KeyMsg:
		switch typed.String() {
		case "enter":
			if len(t.groups) == 0 {
				return nil
			}
			index := t.table.Cursor()
			if index < 0 || index >= len(t.groups) {
				return nil
			}
			detail, err := renderErrorGroupDetail(t.db, t.groups[index])
			if err != nil {
				t.detail = "failed to load error detail: " + err.Error()
			} else {
				t.detail = detail
			}
			t.expanded = true
			t.Resize(t.width, t.height)
			return nil
		case "esc":
			t.expanded = false
			t.Resize(t.width, t.height)
			return nil
		}
	}

	var cmd tea.Cmd
	t.table, cmd = t.table.Update(msg)
	return cmd
}

func (t *errorsTab) View() string {
	if len(t.groups) == 0 {
		return "No grouped errors."
	}
	if !t.expanded {
		return t.table.View()
	}

	detail := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#EF476F")).
		Padding(1, 2).
		Width(max(30, t.width)).
		Render(t.detail)

	return lipgloss.JoinVertical(lipgloss.Left, t.table.View(), "", detail)
}

func renderErrorGroupDetail(db *sql.DB, group store.ErrorGroup) (string, error) {
	lines := []string{
		fmt.Sprintf("Type: %s", group.ExcType),
		fmt.Sprintf("Fingerprint: %s", group.Fingerprint),
		fmt.Sprintf("Message: %s", group.MessageNorm),
		fmt.Sprintf("Count: %d", group.Count),
		fmt.Sprintf("Services: %s", strings.Join(group.Services, ", ")),
	}

	if group.SampleSpanID == "" {
		return strings.Join(lines, "\n"), nil
	}

	span, err := store.GetSpanByID(contextBackground(), db, group.SampleSpanID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return strings.Join(lines, "\n"), nil
		}
		return "", err
	}

	lines = append(lines, "", fmt.Sprintf("Sample span: %s (%s)", span.Name, span.SpanID))
	stack := exceptionStackTrace(span.Events)
	if stack != "" {
		lines = append(lines, "Stack trace:", stack)
	} else {
		lines = append(lines, "Sample payload:", prettyJSON(span))
	}

	return strings.Join(lines, "\n"), nil
}

func exceptionStackTrace(raw string) string {
	if raw == "" {
		return ""
	}

	var events []map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &events); err != nil {
		return ""
	}

	for _, event := range events {
		name, _ := event["name"].(string)
		if name != "exception" {
			continue
		}
		attributes, _ := event["attributes"].(map[string]interface{})
		stackTrace, _ := attributes["exception.stacktrace"].(string)
		if stackTrace != "" {
			return stackTrace
		}
	}

	return ""
}

func prettyJSON(value interface{}) string {
	bytes, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(bytes)
}
