package api

import (
	"net/http"

	"github.com/Meru143/lantern/internal/store"
)

func (s *serverState) listLogs(w http.ResponseWriter, r *http.Request) {
	limit, err := parsePositiveInt(r.URL.Query().Get("limit"), 200)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid limit")
		return
	}
	offset, err := parsePositiveInt(r.URL.Query().Get("offset"), 0)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid offset")
		return
	}
	severity, err := parsePositiveInt(r.URL.Query().Get("severity"), 0)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid severity")
		return
	}

	logs, err := store.ListLogs(r.Context(), s.db, store.LogFilter{
		ServiceName: r.URL.Query().Get("service"),
		MinSeverity: int32(severity),
		TraceID:     r.URL.Query().Get("traceId"),
		Search:      r.URL.Query().Get("search"),
		Limit:       limit,
		Offset:      offset,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, logs)
}
