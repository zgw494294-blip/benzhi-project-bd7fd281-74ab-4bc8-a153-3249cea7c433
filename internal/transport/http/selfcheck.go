package httptransport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type selfResponse struct {
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}
type resultEnvelope struct {
	Dataset struct {
		ID       string `json:"id"`
		Revision int64  `json:"revision"`
		Status   string `json:"status"`
	} `json:"dataset"`
	Data json.RawMessage `json:"data"`
}

func RunSelfCheck(ctx context.Context, baseURL string) error {
	client := &http.Client{Timeout: 3 * time.Second}
	datasetID := "selfcheck-dataset"
	requestNo := 0
	call := func(method, path, role string, revision int64, body any) (resultEnvelope, error) {
		requestNo++
		raw, _ := json.Marshal(body)
		request, err := http.NewRequestWithContext(ctx, method, baseURL+path, bytes.NewReader(raw))
		if err != nil {
			return resultEnvelope{}, err
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Role", role)
		request.Header.Set("X-Actor", "selfcheck-"+role)
		request.Header.Set("X-Request-Id", fmt.Sprintf("selfcheck-%02d", requestNo))
		if revision > 0 {
			request.Header.Set("If-Match-Revision", fmt.Sprint(revision))
		}
		response, err := client.Do(request)
		if err != nil {
			return resultEnvelope{}, err
		}
		defer response.Body.Close()
		var envelope selfResponse
		if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
			return resultEnvelope{}, err
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			if envelope.Error != nil {
				return resultEnvelope{}, fmt.Errorf("%s: %s", envelope.Error.Code, envelope.Error.Message)
			}
			return resultEnvelope{}, fmt.Errorf("HTTP %d", response.StatusCode)
		}
		var result resultEnvelope
		if err := json.Unmarshal(envelope.Data, &result); err != nil {
			return resultEnvelope{}, err
		}
		return result, nil
	}
	get := func(path string, target any) error {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
		if err != nil {
			return err
		}
		response, err := client.Do(request)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		var envelope selfResponse
		if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
			return err
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			if envelope.Error != nil {
				return fmt.Errorf("%s: %s", envelope.Error.Code, envelope.Error.Message)
			}
			return fmt.Errorf("HTTP %d", response.StatusCode)
		}
		return json.Unmarshal(envelope.Data, target)
	}
	created, err := call("POST", "/api/v1/datasets", "manager", 0, map[string]any{"id": datasetID, "title": "回环自检声纹集", "researchGoal": "验证完整研究放行流程", "targetTaxa": []string{"aves"}, "recordingRegion": "selfcheck-region", "qualityRuleVersion": "bio-v1"})
	if err != nil {
		return fmt.Errorf("创建数据集: %w", err)
	}
	revision := created.Dataset.Revision
	registered, err := call("POST", "/api/v1/datasets/"+datasetID+"/samples", "manager", revision, map[string]any{"samples": []map[string]any{{"id": "selfcheck-sample", "sourceRef": "field/selfcheck.wav", "capturedAt": "2024-05-01T00:00:00Z", "latitudeBand": "N30-N31", "longitudeBand": "E120-E121", "sampleRateHz": 48000, "channels": 1, "durationMs": 5000, "sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}}})
	if err != nil {
		return fmt.Errorf("登记样本: %w", err)
	}
	revision = registered.Dataset.Revision
	assessed, err := call("POST", "/api/v1/datasets/"+datasetID+"/assessments/batch", "annotator", revision, map[string]any{"items": []map[string]any{{"sampleId": "selfcheck-sample", "signalToNoiseDb": 24, "clippingRatio": 0.001, "silenceRatio": 0.1}}})
	if err != nil {
		return fmt.Errorf("信号检查: %w", err)
	}
	revision = assessed.Dataset.Revision
	annotated, err := call("POST", "/api/v1/datasets/"+datasetID+"/annotations", "annotator", revision, map[string]any{"sampleId": "selfcheck-sample", "segments": []map[string]int64{{"startMs": 100, "endMs": 2500}}, "speciesCode": "PASSER_MONTANUS", "callType": "contact_call", "confidence": 0.96, "evidenceNote": "自检证据完整"})
	if err != nil {
		return fmt.Errorf("提交标注: %w", err)
	}
	revision = annotated.Dataset.Revision
	var history struct {
		Total int `json:"total"`
	}
	if err := get("/api/v1/datasets/"+datasetID+"/samples/selfcheck-sample/annotations?limit=10&offset=0", &history); err != nil || history.Total != 1 {
		return fmt.Errorf("查询标注履历: total=%d err=%w", history.Total, err)
	}
	var issues struct {
		Total int `json:"total"`
	}
	if err := get("/api/v1/datasets/"+datasetID+"/issues?status=closed&limit=10", &issues); err != nil || issues.Total != 0 {
		return fmt.Errorf("查询专家队列: total=%d err=%w", issues.Total, err)
	}
	var readiness struct {
		Ready          bool   `json:"ready"`
		ManifestDigest string `json:"manifestDigest"`
	}
	if err := get("/api/v1/datasets/"+datasetID+"/freeze/readiness", &readiness); err != nil || !readiness.Ready || readiness.ManifestDigest == "" {
		return fmt.Errorf("查询冻结就绪度: ready=%t err=%w", readiness.Ready, err)
	}
	frozen, err := call("POST", "/api/v1/datasets/"+datasetID+"/freeze", "lead", revision, map[string]any{})
	if err != nil {
		return fmt.Errorf("冻结候选: %w", err)
	}
	if frozen.Dataset.Status != "frozen" {
		return fmt.Errorf("冻结状态异常: %s", frozen.Dataset.Status)
	}
	var manifest struct {
		Total       int  `json:"total"`
		DigestValid bool `json:"digestValid"`
	}
	if err := get("/api/v1/datasets/"+datasetID+"/freeze/items?limit=1", &manifest); err != nil || manifest.Total != 1 || !manifest.DigestValid {
		return fmt.Errorf("复核冻结清单: total=%d valid=%t err=%w", manifest.Total, manifest.DigestValid, err)
	}
	revision = frozen.Dataset.Revision
	released, err := call("POST", "/api/v1/datasets/"+datasetID+"/approve", "lead", revision, map[string]any{})
	if err != nil {
		return fmt.Errorf("批准放行: %w", err)
	}
	if released.Dataset.Status != "released" {
		return fmt.Errorf("最终状态异常: %s", released.Dataset.Status)
	}
	if err := get("/api/v1/datasets/"+datasetID+"/freeze/items", &manifest); err != nil || !manifest.DigestValid {
		return fmt.Errorf("复核放行清单: %w", err)
	}
	return nil
}
