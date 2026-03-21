package api

import (
	"fmt"
	"net/http"
	"sort"
)

type servicesResponse struct {
	Services []string      `json:"services"`
	Edges    []serviceEdge `json:"edges"`
}

type serviceEdge struct {
	From      string  `json:"from"`
	To        string  `json:"to"`
	Count     int     `json:"count"`
	ErrorRate float64 `json:"errorRate"`
}

func (s *serverState) listServices(w http.ResponseWriter, r *http.Request) {
	serviceRows, err := s.db.QueryContext(
		r.Context(),
		`SELECT DISTINCT service_name
		 FROM spans
		 WHERE service_name <> ''
		 ORDER BY service_name ASC`,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("list services: %v", err))
		return
	}
	defer serviceRows.Close()

	var services []string
	for serviceRows.Next() {
		var service string
		if err := serviceRows.Scan(&service); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("scan service: %v", err))
			return
		}
		services = append(services, service)
	}
	if err := serviceRows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("iterate services: %v", err))
		return
	}

	edgeRows, err := s.db.QueryContext(
		r.Context(),
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
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("list service edges: %v", err))
		return
	}
	defer edgeRows.Close()

	var edges []serviceEdge
	for edgeRows.Next() {
		var edge serviceEdge
		if err := edgeRows.Scan(&edge.From, &edge.To, &edge.Count, &edge.ErrorRate); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("scan edge: %v", err))
			return
		}
		edges = append(edges, edge)
	}
	if err := edgeRows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("iterate edges: %v", err))
		return
	}

	sort.Strings(services)
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].Count == edges[j].Count {
			if edges[i].From == edges[j].From {
				return edges[i].To < edges[j].To
			}
			return edges[i].From < edges[j].From
		}
		return edges[i].Count > edges[j].Count
	})

	writeJSON(w, http.StatusOK, servicesResponse{
		Services: services,
		Edges:    edges,
	})
}
