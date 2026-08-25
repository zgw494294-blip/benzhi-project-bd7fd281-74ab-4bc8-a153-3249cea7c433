package httptransport

import (
	"net/http"
	"strings"

	"bioacoustic-release-hub/internal/application"
	"bioacoustic-release-hub/internal/domain"
)

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) CreateDataset(w http.ResponseWriter, r *http.Request) {
	var cmd application.CreateCommand
	if err := decodeJSON(w, r, &cmd); err != nil {
		handleError(w, err)
		return
	}
	role, err := domain.ParseRole(r.Header.Get("X-Role"))
	if err != nil {
		handleError(w, err)
		return
	}
	cmd.Role = role
	cmd.Actor = strings.TrimSpace(r.Header.Get("X-Actor"))
	cmd.RequestID = strings.TrimSpace(r.Header.Get("X-Request-Id"))
	result, err := h.service.CreateDataset(r.Context(), cmd)
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h *Handler) RegisterSamples(w http.ResponseWriter, r *http.Request) {
	meta, err := commandMetadata(r)
	if err != nil {
		handleError(w, err)
		return
	}
	var body struct {
		Samples []application.SampleDraft `json:"samples"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		handleError(w, err)
		return
	}
	result, err := h.service.RegisterSamples(r.Context(), application.RegisterCommand{Metadata: meta, Samples: body.Samples})
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) AssessSignal(w http.ResponseWriter, r *http.Request) {
	meta, err := commandMetadata(r)
	if err != nil {
		handleError(w, err)
		return
	}
	var cmd application.AssessCommand
	if err := decodeJSON(w, r, &cmd); err != nil {
		handleError(w, err)
		return
	}
	cmd.Metadata = meta
	result, err := h.service.AssessSignal(r.Context(), cmd)
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) AssessSignals(w http.ResponseWriter, r *http.Request) {
	meta, err := commandMetadata(r)
	if err != nil {
		handleError(w, err)
		return
	}
	var cmd application.BatchAssessCommand
	if err := decodeJSON(w, r, &cmd); err != nil {
		handleError(w, err)
		return
	}
	cmd.Metadata = meta
	result, err := h.service.AssessSignals(r.Context(), cmd)
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) SubmitAnnotation(w http.ResponseWriter, r *http.Request) {
	meta, err := commandMetadata(r)
	if err != nil {
		handleError(w, err)
		return
	}
	var cmd application.AnnotateCommand
	if err := decodeJSON(w, r, &cmd); err != nil {
		handleError(w, err)
		return
	}
	cmd.Metadata = meta
	result, err := h.service.SubmitAnnotation(r.Context(), cmd)
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) DecideIssue(w http.ResponseWriter, r *http.Request) {
	meta, err := commandMetadata(r)
	if err != nil {
		handleError(w, err)
		return
	}
	prefix := "/api/v1/datasets/" + meta.DatasetID + "/issues/"
	issueID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, prefix), "/decision")
	var body struct {
		Decision domain.IssueDecision `json:"decision"`
		Note     string               `json:"note"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		handleError(w, err)
		return
	}
	result, err := h.service.DecideIssue(r.Context(), application.DecideCommand{Metadata: meta, IssueID: issueID, Decision: body.Decision, Note: body.Note})
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) FreezeCandidate(w http.ResponseWriter, r *http.Request) {
	meta, err := commandMetadata(r)
	if err != nil {
		handleError(w, err)
		return
	}
	if err := decodeEmptyObject(w, r); err != nil {
		handleError(w, err)
		return
	}
	result, err := h.service.Freeze(r.Context(), application.FreezeCommand{Metadata: meta})
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) RevokeCandidate(w http.ResponseWriter, r *http.Request) {
	meta, err := commandMetadata(r)
	if err != nil {
		handleError(w, err)
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		handleError(w, err)
		return
	}
	result, err := h.service.Revoke(r.Context(), application.RevokeCommand{Metadata: meta, Reason: body.Reason})
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) ApproveRelease(w http.ResponseWriter, r *http.Request) {
	meta, err := commandMetadata(r)
	if err != nil {
		handleError(w, err)
		return
	}
	if err := decodeEmptyObject(w, r); err != nil {
		handleError(w, err)
		return
	}
	result, err := h.service.Approve(r.Context(), application.ApproveCommand{Metadata: meta})
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func decodeEmptyObject(w http.ResponseWriter, r *http.Request) error {
	var body struct{}
	return decodeJSON(w, r, &body)
}
