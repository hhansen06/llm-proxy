package handlers

import (
	"encoding/json"
	"net/http"
)

func (h *Handlers) Healthz(w http.ResponseWriter, r *http.Request) {
	type health struct {
		Status string `json:"status"`
		Env    string `json:"env"`
	}
	_ = json.NewEncoder(w).Encode(health{Status: "ok", Env: h.cfg.AppEnv})
}
