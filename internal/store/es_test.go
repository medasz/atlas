package store

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestESGetNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	es := NewES(srv.URL, "assets")
	if _, err := es.Get(context.Background(), "host:1.2.3.4"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestESUpdateAsset_PayloadAndDocImmutability(t *testing.T) {
	var capturedPayload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&capturedPayload); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"result":"updated"}`))
	}))
	defer srv.Close()

	es := NewES(srv.URL, "assets")
	inputDoc := map[string]any{
		"ip":         "127.0.0.1",
		"doc_type":   "host",
		"first_seen": "2026-07-29T18:00:00Z",
	}
	snapshotDoc := map[string]any{
		"ip":         "127.0.0.1",
		"doc_type":   "host",
		"first_seen": "2026-07-29T18:00:00Z",
	}

	err := es.UpdateAsset(context.Background(), "host:127.0.0.1", inputDoc)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// 1. 使用 reflect.DeepEqual 断言调用后所有键和值完全不变
	if !reflect.DeepEqual(inputDoc, snapshotDoc) {
		t.Errorf("expected inputDoc to remain unchanged, got %v, want %v", inputDoc, snapshotDoc)
	}

	// 2. 断言请求体不包含顶层 "doc" 键（避免与 script 冲突）
	if _, ok := capturedPayload["doc"]; ok {
		t.Errorf("request payload must NOT contain top-level 'doc' field when using script")
	}

	// 3. 断言包含 scripted_upsert 和 upsert 键
	if su, ok := capturedPayload["scripted_upsert"].(bool); !ok || !su {
		t.Errorf("expected 'scripted_upsert': true, got %v", capturedPayload["scripted_upsert"])
	}

	// 4. 断言 upsert 与输入 doc 完整等价且包含 first_seen
	upsert, ok := capturedPayload["upsert"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'upsert' map in payload")
	}
	if !reflect.DeepEqual(upsert, snapshotDoc) {
		t.Errorf("expected upsert map %v to match snapshotDoc %v", upsert, snapshotDoc)
	}

	// 5. 解码并断言 script.params 结构与 first_seen 隔离
	script, ok := capturedPayload["script"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'script' map in payload")
	}
	params, ok := script["params"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'params' map in script")
	}
	if fs, ok := params["fs"].(string); !ok || fs != "2026-07-29T18:00:00Z" {
		t.Errorf("expected params.fs to be '2026-07-29T18:00:00Z', got %v", params["fs"])
	}

	paramsDoc, ok := params["doc"].(map[string]any)
	if !ok {
		t.Fatalf("expected params.doc map in script params")
	}
	if _, hasFS := paramsDoc["first_seen"]; hasFS {
		t.Errorf("params.doc must NOT contain 'first_seen' field")
	}
	if paramsDoc["ip"] != "127.0.0.1" || paramsDoc["doc_type"] != "host" {
		t.Errorf("params.doc missing expected fields, got %v", paramsDoc)
	}
}

func TestESUpdateAsset_WithoutFirstSeen(t *testing.T) {
	var capturedPayload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&capturedPayload); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"result":"updated"}`))
	}))
	defer srv.Close()

	es := NewES(srv.URL, "assets")
	inputDoc := map[string]any{
		"ip":       "127.0.0.1",
		"doc_type": "host",
		"os":       "Linux",
	}
	snapshotDoc := map[string]any{
		"ip":       "127.0.0.1",
		"doc_type": "host",
		"os":       "Linux",
	}

	err := es.UpdateAsset(context.Background(), "host:127.0.0.1", inputDoc)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// 1. 输入 doc 深等价不变性断言
	if !reflect.DeepEqual(inputDoc, snapshotDoc) {
		t.Errorf("expected inputDoc to remain unchanged, got %v, want %v", inputDoc, snapshotDoc)
	}

	// 2. 顶层不出现 doc 键
	if _, ok := capturedPayload["doc"]; ok {
		t.Errorf("request payload must NOT contain top-level 'doc' field")
	}

	// 3. scripted_upsert 为 true
	if su, ok := capturedPayload["scripted_upsert"].(bool); !ok || !su {
		t.Errorf("expected 'scripted_upsert': true, got %v", capturedPayload["scripted_upsert"])
	}

	// 4. upsert 包含全部字段
	upsert, ok := capturedPayload["upsert"].(map[string]any)
	if !ok || !reflect.DeepEqual(upsert, snapshotDoc) {
		t.Errorf("expected upsert %v to match snapshot %v", upsert, snapshotDoc)
	}

	// 5. params 中不出现 fs 键，params.doc 包含全部输入字段
	script, ok := capturedPayload["script"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'script' map in payload")
	}
	params, ok := script["params"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'params' map in script")
	}

	if _, hasFS := params["fs"]; hasFS {
		t.Errorf("script.params must NOT contain 'fs' when first_seen is absent")
	}

	paramsDoc, ok := params["doc"].(map[string]any)
	if !ok || !reflect.DeepEqual(paramsDoc, snapshotDoc) {
		t.Errorf("expected params.doc %v to contain all input fields %v", paramsDoc, snapshotDoc)
	}
}

func TestESUpdateAsset_ErrorReasonExposed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"root_cause":[{"type":"action_request_validation_exception","reason":"Validation Failed: 1: can't provide both script and doc;"}],"type":"action_request_validation_exception","reason":"Validation Failed: 1: can't provide both script and doc;"},"status":400}`))
	}))
	defer srv.Close()

	es := NewES(srv.URL, "assets")
	err := es.UpdateAsset(context.Background(), "host:127.0.0.1", map[string]any{"ip": "127.0.0.1"})
	if err == nil {
		t.Fatalf("expected error on 400 status")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "Validation Failed: 1: can't provide both script and doc;") {
		t.Errorf("expected error message to contain ES reason, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "400") || !strings.Contains(errMsg, "host:127.0.0.1") {
		t.Errorf("expected error message to retain status and id, got: %s", errMsg)
	}
}
