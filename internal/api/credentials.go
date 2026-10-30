package api

import "net/http"

func (a API) rotateCredential(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Value string `json:"value"`
	}
	if err := body(w, r, &req); err != nil {
		fail(w, r, err)
		return
	}
	if err := a.Store.RotateCredential(r.Context(), r.PathValue("workspace"), r.PathValue("credential"), req.Value, a.Box); err != nil {
		fail(w, r, err)
		return
	}
	respond(w, 200, map[string]bool{"rotated": true})
}
func (a API) revokeCredential(w http.ResponseWriter, r *http.Request) {
	if err := a.Store.RevokeCredential(r.Context(), r.PathValue("workspace"), r.PathValue("credential")); err != nil {
		fail(w, r, err)
		return
	}
	respond(w, 200, map[string]bool{"revoked": true})
}
