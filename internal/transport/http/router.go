package httptransport

import (
	"net/http"
	"strings"

	"bioacoustic-release-hub/internal/application"
)

type Handler struct {
	service *application.Service
	mux     *http.ServeMux
}

func NewHandler(service *application.Service) *Handler {
	h := &Handler{service: service, mux: http.NewServeMux()}
	h.mux.HandleFunc("GET /healthz", h.Health)
	h.mux.HandleFunc("POST /api/v1/datasets", h.CreateDataset)
	h.mux.HandleFunc("GET /api/v1/credentials/{code}/verify", h.VerifyCredential)
	h.mux.HandleFunc("/api/v1/datasets/{id}", h.DatasetRoutes)
	h.mux.HandleFunc("/api/v1/datasets/{id}/{rest...}", h.DatasetRoutes)
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	h.mux.ServeHTTP(w, r)
}

func (h *Handler) DatasetRoutes(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	suffix := strings.TrimPrefix(r.URL.Path, "/api/v1/datasets/"+id)
	switch {
	case suffix == "" && r.Method == http.MethodGet:
		h.GetDataset(w, r)
	case suffix == "/samples" && r.Method == http.MethodPost:
		h.RegisterSamples(w, r)
	case suffix == "/samples" && r.Method == http.MethodGet:
		h.ListSamples(w, r)
	case suffix == "/assessments" && r.Method == http.MethodPost:
		h.AssessSignal(w, r)
	case suffix == "/assessments/batch" && r.Method == http.MethodPost:
		h.AssessSignals(w, r)
	case suffix == "/annotations" && r.Method == http.MethodPost:
		h.SubmitAnnotation(w, r)
	case strings.HasPrefix(suffix, "/samples/") && strings.HasSuffix(suffix, "/annotations") && r.Method == http.MethodGet:
		h.AnnotationHistory(w, r)
	case suffix == "/issues" && r.Method == http.MethodGet:
		h.ListIssues(w, r)
	case strings.HasPrefix(suffix, "/issues/") && strings.HasSuffix(suffix, "/decision") && r.Method == http.MethodPost:
		h.DecideIssue(w, r)
	case suffix == "/freeze" && r.Method == http.MethodPost:
		h.FreezeCandidate(w, r)
	case suffix == "/freeze/readiness" && r.Method == http.MethodGet:
		h.FreezeReadiness(w, r)
	case suffix == "/freeze/items" && r.Method == http.MethodGet:
		h.FrozenItems(w, r)
	case suffix == "/revoke" && r.Method == http.MethodPost:
		h.RevokeCandidate(w, r)
	case suffix == "/approve" && r.Method == http.MethodPost:
		h.ApproveRelease(w, r)
	case suffix == "/timeline" && r.Method == http.MethodGet:
		h.Timeline(w, r)
	case suffix == "/credential" && r.Method == http.MethodGet:
		h.GetCredential(w, r)
	default:
		writeError(w, http.StatusNotFound, "NOT_FOUND", "路由不存在", nil)
	}
}
