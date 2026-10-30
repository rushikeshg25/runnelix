package api

import (
	"github.com/rushikeshg25/runnelix/internal/store"
	"net/http"
	"strconv"
)

func (a API) auditRecords(w http.ResponseWriter, r *http.Request) {
	var after int64
	if raw := r.URL.Query().Get("after"); raw != "" {
		var err error
		after, err = strconv.ParseInt(raw, 10, 64)
		if err != nil {
			fail(w, r, store.ErrInvalid)
			return
		}
	}
	records, err := a.Store.AuditRecords(r.Context(), r.PathValue("workspace"), after)
	if err != nil {
		fail(w, r, err)
		return
	}
	respond(w, 200, records)
}
