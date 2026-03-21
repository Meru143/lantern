package tui

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type servicesTab struct {
	db      *sql.DB
	width   int
	height  int
	content string
}

type serviceGraph struct {
	services []string
	edges    []serviceEdgeSummary
}

type serviceEdgeSummary struct {
	from      string
	to        string
	count     int
	errorRate float64
}

func newServicesTab(db *sql.DB) *servicesTab {
	return &servicesTab{db: db}
}

func (t *servicesTab) Title() string {
	return "Services"
}

func (t *servicesTab) Refresh() error {
	graph, err := queryServiceGraph(t.db)
	if err != nil {
		return err
	}

	if len(graph.services) == 0 {
		t.content = "No service relationships found yet."
		return nil
	}

	lines := []string{"Service dependency graph", strings.Repeat("-", 24)}
	for _, service := range graph.services {
		lines = append(lines, service)
		for _, edge := range graph.edges {
			if edge.from != service {
				continue
			}
			lines = append(lines, fmt.Sprintf("  -> %s (%d calls, %.1f%% errors)", edge.to, edge.count, edge.errorRate))
		}
	}

	t.content = strings.Join(lines, "\n")
	return nil
}

func (t *servicesTab) Resize(width, height int) {
	t.width = width
	t.height = height
}

func (t *servicesTab) Update(tea.Msg) tea.Cmd {
	return nil
}

func (t *servicesTab) View() string {
	return t.content
}

func queryServiceGraph(db *sql.DB) (serviceGraph, error) {
	rows, err := db.QueryContext(
		contextBackground(),
		`SELECT DISTINCT service_name
		 FROM spans
		 WHERE service_name <> ''
		 ORDER BY service_name ASC`,
	)
	if err != nil {
		return serviceGraph{}, err
	}
	defer rows.Close()

	graph := serviceGraph{}
	for rows.Next() {
		var service string
		if err := rows.Scan(&service); err != nil {
			return serviceGraph{}, err
		}
		graph.services = append(graph.services, service)
	}
	if err := rows.Err(); err != nil {
		return serviceGraph{}, err
	}

	edgeRows, err := db.QueryContext(
		contextBackground(),
		`SELECT parent.service_name, child.service_name, COUNT(*),
		        AVG(CASE WHEN child.status_code <> 0 THEN 100.0 ELSE 0.0 END)
		 FROM spans child
		 JOIN spans parent ON parent.span_id = child.parent_span_id
		 WHERE parent.service_name <> ''
		   AND child.service_name <> ''
		   AND parent.service_name <> child.service_name
		 GROUP BY parent.service_name, child.service_name`,
	)
	if err != nil {
		return serviceGraph{}, err
	}
	defer edgeRows.Close()

	for edgeRows.Next() {
		var edge serviceEdgeSummary
		if err := edgeRows.Scan(&edge.from, &edge.to, &edge.count, &edge.errorRate); err != nil {
			return serviceGraph{}, err
		}
		graph.edges = append(graph.edges, edge)
	}
	if err := edgeRows.Err(); err != nil {
		return serviceGraph{}, err
	}

	sort.Strings(graph.services)
	sort.Slice(graph.edges, func(i, j int) bool {
		if graph.edges[i].from == graph.edges[j].from {
			return graph.edges[i].to < graph.edges[j].to
		}
		return graph.edges[i].from < graph.edges[j].from
	})

	return graph, nil
}
