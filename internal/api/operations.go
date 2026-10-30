package api

import (
	"context"
	"net/http"
	"time"
)

func (a API) operationalMetrics(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	m, err := a.Store.OperationalMetrics(ctx, r.PathValue("workspace"))
	if err != nil {
		respond(w, http.StatusServiceUnavailable, map[string]string{"code": "metrics_unavailable", "message": "Operational metrics unavailable"})
		return
	}
	respond(w, http.StatusOK, m)
}
