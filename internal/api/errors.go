package api

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/Meru143/lantern/internal/store"
	"github.com/go-chi/chi/v5"
)

type errorDetailResponse struct {
	Group      *store.ErrorGroup `json:"group"`
	SampleSpan *store.SpanRecord `json:"sampleSpan"`
}

func (s *serverState) listErrors(w http.ResponseWriter, r *http.Request) {
	groups, err := store.ListErrorGroups(r.Context(), s.db)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, groups)
}

func (s *serverState) getError(w http.ResponseWriter, r *http.Request) {
	fingerprint := chi.URLParam(r, "fingerprint")
	if fingerprint == "" {
		writeError(w, http.StatusBadRequest, "fingerprint is required")
		return
	}

	group, err := store.GetErrorGroup(r.Context(), s.db, fingerprint)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "error group not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var sampleSpan *store.SpanRecord
	if group.SampleSpanID != "" {
		sampleSpan, err = store.GetSpanByID(r.Context(), s.db, group.SampleSpanID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			sampleSpan = nil
		}
	}

	writeJSON(w, http.StatusOK, errorDetailResponse{
		Group:      group,
		SampleSpan: sampleSpan,
	})
}
