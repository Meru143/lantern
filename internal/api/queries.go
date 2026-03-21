package api

import (
	"net/http"

	"github.com/Meru143/lantern/internal/store"
)

func (s *serverState) listQueries(w http.ResponseWriter, r *http.Request) {
	stats, err := store.ListSlowQueries(r.Context(), s.db, 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, stats)
}
