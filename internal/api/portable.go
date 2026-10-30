package api

import (
	"github.com/rushikeshg25/runnelix/internal/definition"
	"net/http"
)

func (a API) exportWorkflow(w http.ResponseWriter, r *http.Request) {
	draft, err := a.Store.GetWorkflow(r.Context(), r.PathValue("workspace"), r.PathValue("workflow"))
	if err != nil {
		fail(w, r, err)
		return
	}
	bundle, err := definition.ExportPortable(draft.Name, draft.Graph, a.Store.Catalog)
	if err != nil {
		fail(w, r, err)
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="workflow.json"`)
	respond(w, 200, bundle)
}
func (a API) importWorkflow(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Bundle   definition.PortableWorkflow `json:"bundle"`
		Bindings map[string]string           `json:"bindings"`
	}
	if err := body(w, r, &req); err != nil {
		fail(w, r, err)
		return
	}
	workflow, err := a.Store.ImportWorkflow(r.Context(), r.PathValue("workspace"), req.Bundle, req.Bindings)
	if err != nil {
		fail(w, r, err)
		return
	}
	w.Header().Set("ETag", `"1"`)
	respond(w, 201, workflow)
}
