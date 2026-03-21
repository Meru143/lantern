package api

import (
	"net/http"
	"time"

	"github.com/Meru143/lantern/internal/store"
	"github.com/go-chi/chi/v5"
)

type traceDetailResponse struct {
	TraceID string             `json:"traceId"`
	Spans   []spanNode         `json:"tree"`
	Flat    []store.SpanRecord `json:"spans"`
}

type spanNode struct {
	Span     store.SpanRecord `json:"span"`
	Children []spanNode       `json:"children"`
}

type linkedSpanNode struct {
	Span     store.SpanRecord
	Children []*linkedSpanNode
}

func (s *serverState) listTraces(w http.ResponseWriter, r *http.Request) {
	minDuration := time.Duration(0)
	if rawDuration := r.URL.Query().Get("minDuration"); rawDuration != "" {
		parsed, err := time.ParseDuration(rawDuration)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid minDuration")
			return
		}
		minDuration = parsed
	}

	limit, err := parsePositiveInt(r.URL.Query().Get("limit"), 100)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid limit")
		return
	}
	offset, err := parsePositiveInt(r.URL.Query().Get("offset"), 0)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid offset")
		return
	}

	traces, err := store.ListTraces(r.Context(), s.db, store.TraceFilter{
		ServiceName: r.URL.Query().Get("service"),
		HasError:    r.URL.Query().Get("hasError") == "true",
		MinDuration: minDuration,
		Limit:       limit,
		Offset:      offset,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, traces)
}

func (s *serverState) getTrace(w http.ResponseWriter, r *http.Request) {
	traceID := chi.URLParam(r, "traceId")
	if traceID == "" {
		writeError(w, http.StatusBadRequest, "traceId is required")
		return
	}

	spans, err := store.GetTrace(r.Context(), s.db, traceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(spans) == 0 {
		writeError(w, http.StatusNotFound, "trace not found")
		return
	}

	writeJSON(w, http.StatusOK, traceDetailResponse{
		TraceID: traceID,
		Spans:   buildTraceTree(spans),
		Flat:    spans,
	})
}

func buildTraceTree(spans []store.SpanRecord) []spanNode {
	nodes := make(map[string]*linkedSpanNode, len(spans))
	roots := make([]*linkedSpanNode, 0)

	for _, span := range spans {
		clone := span
		nodes[clone.SpanID] = &linkedSpanNode{Span: clone}
	}

	for _, span := range spans {
		node := nodes[span.SpanID]
		if span.ParentSpanID == "" {
			roots = append(roots, node)
			continue
		}

		parent, ok := nodes[span.ParentSpanID]
		if !ok {
			roots = append(roots, node)
			continue
		}
		parent.Children = append(parent.Children, node)
	}

	result := make([]spanNode, 0, len(roots))
	for _, root := range roots {
		result = append(result, materializeNode(root))
	}
	return result
}

func materializeNode(node *linkedSpanNode) spanNode {
	if node == nil {
		return spanNode{}
	}

	children := make([]spanNode, 0, len(node.Children))
	for _, child := range node.Children {
		children = append(children, materializeNode(child))
	}

	return spanNode{
		Span:     node.Span,
		Children: children,
	}
}
