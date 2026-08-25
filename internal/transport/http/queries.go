package httptransport

import (
	"net/http"
	"strings"

	"bioacoustic-release-hub/internal/application"
	"bioacoustic-release-hub/internal/domain"
)

func (h *Handler) GetDataset(w http.ResponseWriter, r *http.Request) {
	view, err := h.service.GetDataset(r.Context(), r.PathValue("id"))
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}
func (h *Handler) ListSamples(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListSamples(r.Context(), r.PathValue("id"), pageFrom(r))
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "count": len(items)})
}
func (h *Handler) ListIssues(w http.ResponseWriter, r *http.Request) {
	page, err := validatedPageFrom(r)
	if err != nil {
		handleError(w, err)
		return
	}
	status, err := singleQueryValue(r, "status")
	if err != nil {
		handleError(w, err)
		return
	}
	kind, err := singleQueryValue(r, "kind")
	if err != nil {
		handleError(w, err)
		return
	}
	severity, err := singleQueryValue(r, "severity")
	if err != nil {
		handleError(w, err)
		return
	}
	sampleID, err := singleQueryValue(r, "sampleId")
	if err != nil {
		handleError(w, err)
		return
	}
	if r.URL.Query().Has("sampleId") && strings.TrimSpace(sampleID) == "" {
		handleError(w, domain.NewError(domain.CodeInvalid, "sampleId 不能为空"))
		return
	}
	queue, err := h.service.QueryIssues(r.Context(), r.PathValue("id"), application.IssueFilter{Status: status, Kind: kind, Severity: severity, SampleID: sampleID}, page)
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, queue)
}

func (h *Handler) FreezeReadiness(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.FreezeReadiness(r.Context(), r.PathValue("id"))
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) AnnotationHistory(w http.ResponseWriter, r *http.Request) {
	page, err := validatedPageFrom(r)
	if err != nil {
		handleError(w, err)
		return
	}
	prefix := "/api/v1/datasets/" + r.PathValue("id") + "/samples/"
	sampleID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, prefix), "/annotations")
	if sampleID == "" || strings.Contains(sampleID, "/") {
		handleError(w, domain.NewError(domain.CodeNotFound, "样本不存在"))
		return
	}
	result, err := h.service.AnnotationHistory(r.Context(), r.PathValue("id"), sampleID, page)
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) FrozenItems(w http.ResponseWriter, r *http.Request) {
	page, err := validatedPageFrom(r)
	if err != nil {
		handleError(w, err)
		return
	}
	result, err := h.service.FrozenItems(r.Context(), r.PathValue("id"), page)
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (h *Handler) Timeline(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.Timeline(r.Context(), r.PathValue("id"), pageFrom(r))
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "count": len(items)})
}
func (h *Handler) GetCredential(w http.ResponseWriter, r *http.Request) {
	credential, err := h.service.GetCredential(r.Context(), r.PathValue("id"))
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, credential)
}
func (h *Handler) VerifyCredential(w http.ResponseWriter, r *http.Request) {
	credential, valid, err := h.service.VerifyCredential(r.Context(), r.PathValue("code"))
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"valid": valid, "credential": credential})
}
