package httptransport

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"bioacoustic-release-hub/internal/application"
	"bioacoustic-release-hub/internal/domain"
)

const maxBodyBytes = 1 << 20

type envelope struct {
	Data  any        `json:"data,omitempty"`
	Error *errorBody `json:"error,omitempty"`
}
type errorBody struct {
	Code            string               `json:"code"`
	Message         string               `json:"message"`
	CurrentRevision int64                `json:"currentRevision,omitempty"`
	CurrentStatus   domain.DatasetStatus `json:"currentStatus,omitempty"`
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	contentType := r.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType != "application/json" {
		return domain.NewError(domain.CodeInvalid, "Content-Type 必须是 application/json")
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return domain.NewError(domain.CodeInvalid, "JSON 请求体无效: %v", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return domain.NewError(domain.CodeInvalid, "请求体只能包含一个 JSON 对象")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{Data: data})
}
func writeError(w http.ResponseWriter, status int, code, message string, de *domain.Error) {
	body := &errorBody{Code: code, Message: message}
	if de != nil {
		body.CurrentRevision = de.CurrentRevision
		body.CurrentStatus = de.CurrentStatus
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{Error: body})
}

func handleError(w http.ResponseWriter, err error) {
	var de *domain.Error
	if errors.As(err, &de) {
		status := http.StatusBadRequest
		switch de.Code {
		case domain.CodeNotFound:
			status = http.StatusNotFound
		case domain.CodeConflict, domain.CodeStaleRevision:
			status = http.StatusConflict
		case domain.CodeForbidden:
			status = http.StatusForbidden
		case domain.CodeFrozen, domain.CodePrecondition:
			status = http.StatusUnprocessableEntity
		case domain.CodeIntegrity:
			status = http.StatusInternalServerError
		}
		writeError(w, status, string(de.Code), de.Message, de)
		return
	}
	writeError(w, http.StatusInternalServerError, "INTERNAL", "服务内部错误", nil)
}

func commandMetadata(r *http.Request) (application.Metadata, error) {
	role, err := domain.ParseRole(strings.TrimSpace(r.Header.Get("X-Role")))
	if err != nil {
		return application.Metadata{}, err
	}
	revision, err := strconv.ParseInt(r.Header.Get("If-Match-Revision"), 10, 64)
	if err != nil || revision <= 0 {
		return application.Metadata{}, domain.NewError(domain.CodeInvalid, "If-Match-Revision 必须是正整数")
	}
	meta := application.Metadata{DatasetID: r.PathValue("id"), RequestID: strings.TrimSpace(r.Header.Get("X-Request-Id")), Revision: revision, Role: role, Actor: strings.TrimSpace(r.Header.Get("X-Actor"))}
	if meta.RequestID == "" || meta.Actor == "" {
		return meta, domain.NewError(domain.CodeInvalid, "X-Request-Id 和 X-Actor 不能为空")
	}
	return meta, nil
}

func pageFrom(r *http.Request) domain.Page {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	return domain.Page{Limit: limit, Offset: offset}
}

func validatedPageFrom(r *http.Request) (domain.Page, error) {
	page := domain.Page{Limit: 100}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 500 {
			return domain.Page{}, domain.NewError(domain.CodeInvalid, "limit 必须是 1 到 500 的整数")
		}
		page.Limit = limit
	} else if r.URL.Query().Has("limit") {
		return domain.Page{}, domain.NewError(domain.CodeInvalid, "limit 不能为空")
	}
	if raw := r.URL.Query().Get("offset"); raw != "" {
		offset, err := strconv.Atoi(raw)
		if err != nil || offset < 0 {
			return domain.Page{}, domain.NewError(domain.CodeInvalid, "offset 必须是非负整数")
		}
		page.Offset = offset
	} else if r.URL.Query().Has("offset") {
		return domain.Page{}, domain.NewError(domain.CodeInvalid, "offset 不能为空")
	}
	return page, nil
}

func singleQueryValue(r *http.Request, name string) (string, error) {
	values, ok := r.URL.Query()[name]
	if !ok {
		return "", nil
	}
	if len(values) != 1 {
		return "", domain.NewError(domain.CodeInvalid, "%s 不能重复或包含矛盾值", name)
	}
	return values[0], nil
}
