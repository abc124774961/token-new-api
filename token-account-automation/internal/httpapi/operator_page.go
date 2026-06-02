package httpapi

import (
	_ "embed"
	"net/http"
)

//go:embed operator.html
var operatorHTML []byte

func (s *Server) operatorPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(operatorHTML)
}
