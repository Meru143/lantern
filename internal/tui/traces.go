package tui

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/Meru143/lantern/internal/store"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type tracesTab struct {
	db        *sql.DB
	table     table.Model
	width     int
	height    int
	summaries []store.TraceSummary
	detail    string
	showTrace bool
}

func newTracesTab(db *sql.DB) *tracesTab {
	columns := []table.Column{
		{Title: "Service", Width: 18},
		{Title: "Name", Width: 28},
		{Title: "Duration", Width: 12},
		{Title: "Spans", Width: 8},
		{Title: "Error", Width: 8},
	}

	model := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(10),
	)

	styles := table.DefaultStyles()
	styles.Selected = styles.Selected.
		Foreground(lipgloss.Color("#111417")).
		Background(lipgloss.Color("#FFD166")).
		Bold(true)
	model.SetStyles(styles)

	return &tracesTab{
		db:    db,
		table: model,
	}
}

func (t *tracesTab) Title() string {
	return "Traces"
}

func (t *tracesTab) Refresh() error {
	summaries, err := store.ListTraces(contextBackground(), t.db, store.TraceFilter{Limit: 100})
	if err != nil {
		return err
	}

	rows := make([]table.Row, 0, len(summaries))
	for _, summary := range summaries {
		rows = append(rows, table.Row{
			strings.Join(summary.ServiceNames, ","),
			firstNonEmpty(summary.Name, summary.TraceID),
			shortDuration(summary.DurationNanos),
			fmt.Sprintf("%d", summary.SpanCount),
			boolLabel(summary.HasError),
		})
	}

	t.summaries = summaries
	t.table.SetRows(rows)
	if cursor := t.table.Cursor(); cursor >= len(rows) && len(rows) > 0 {
		t.table.SetCursor(len(rows) - 1)
	}
	if len(rows) == 0 {
		t.detail = ""
		t.showTrace = false
	}

	return nil
}

func (t *tracesTab) Resize(width, height int) {
	t.width = width
	t.height = height
	tableHeight := height - 4
	if t.showTrace {
		tableHeight = height / 2
	}
	if tableHeight < 5 {
		tableHeight = 5
	}
	t.table.SetWidth(width)
	t.table.SetHeight(tableHeight)
}

func (t *tracesTab) Update(msg tea.Msg) tea.Cmd {
	switch typed := msg.(type) {
	case tea.KeyMsg:
		switch typed.String() {
		case "enter":
			if len(t.summaries) == 0 {
				return nil
			}
			index := t.table.Cursor()
			if index < 0 || index >= len(t.summaries) {
				return nil
			}

			spans, err := store.GetTrace(contextBackground(), t.db, t.summaries[index].TraceID)
			if err != nil {
				t.detail = "failed to load trace: " + err.Error()
				t.showTrace = true
				t.Resize(t.width, t.height)
				return nil
			}
			t.detail = renderTraceDetail(t.summaries[index], spans, max(30, t.width-6))
			t.showTrace = true
			t.Resize(t.width, t.height)
			return nil
		case "esc":
			t.showTrace = false
			t.Resize(t.width, t.height)
			return nil
		}
	}

	var cmd tea.Cmd
	t.table, cmd = t.table.Update(msg)
	return cmd
}

func (t *tracesTab) View() string {
	if len(t.summaries) == 0 {
		return "No traces yet. Run `lantern serve` and point your OTEL exporter at localhost:4317."
	}

	if !t.showTrace {
		return t.table.View()
	}

	detail := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7BDFF2")).
		Padding(1, 2).
		Width(max(30, t.width)).
		Render(t.detail)

	return lipgloss.JoinVertical(lipgloss.Left, t.table.View(), "", detail)
}

func renderTraceDetail(summary store.TraceSummary, spans []store.SpanRecord, width int) string {
	if len(spans) == 0 {
		return "Trace detail unavailable."
	}

	start := spans[0].StartTimeUnix
	end := spans[0].EndTimeUnix
	depths := make(map[string]int, len(spans))
	for _, span := range spans {
		if span.StartTimeUnix < start {
			start = span.StartTimeUnix
		}
		if span.EndTimeUnix > end {
			end = span.EndTimeUnix
		}
		if span.ParentSpanID != "" {
			depths[span.SpanID] = depths[span.ParentSpanID] + 1
		}
	}

	total := end - start
	if total <= 0 {
		total = 1
	}

	barWidth := width - 34
	if barWidth < 10 {
		barWidth = 10
	}

	lines := []string{
		fmt.Sprintf("%s [%s]", firstNonEmpty(summary.Name, summary.TraceID), summary.TraceID),
		fmt.Sprintf("Duration %s | %d spans | Services %s", shortDuration(summary.DurationNanos), summary.SpanCount, strings.Join(summary.ServiceNames, ", ")),
		"",
	}

	for _, span := range spans {
		duration := span.EndTimeUnix - span.StartTimeUnix
		filled := int((float64(duration) / float64(total)) * float64(barWidth))
		if filled < 1 {
			filled = 1
		}
		if filled > barWidth {
			filled = barWidth
		}

		offset := int((float64(span.StartTimeUnix-start) / float64(total)) * float64(barWidth))
		if offset < 0 {
			offset = 0
		}
		if offset > barWidth-1 {
			offset = barWidth - 1
		}

		bar := strings.Repeat(" ", offset) + strings.Repeat("=", filled)
		bar = padRight(bar, barWidth)
		indent := strings.Repeat("  ", depths[span.SpanID])
		lines = append(lines,
			fmt.Sprintf("%-10s %8s %s%s", firstNonEmpty(span.ServiceName, "unknown"), shortDuration(duration), indent, span.Name),
			fmt.Sprintf("            [%s]", bar),
		)
	}

	return strings.Join(lines, "\n")
}
