package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"benzhi/deacidification/internal/domain"
	"benzhi/deacidification/internal/service"
)

// Handler 包含业务服务
type Handler struct {
	svc *service.BatchService
}

// NewHandler 构造 HTTP 处理器
func NewHandler(svc *service.BatchService) http.Handler {
	h := &Handler{svc: svc}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/items", h.handleItems)
	mux.HandleFunc("/api/detections", h.handleDetections)
	mux.HandleFunc("/api/batches", h.handleBatches)
	mux.HandleFunc("/api/batches/", h.handleBatchByID)
	mux.HandleFunc("/api/items/needing-retest", h.handleNeedingRetest)
	return mux
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decodeJSON(r *http.Request, dst interface{}) error {
	dec := json.NewDecoder(r.Body)
	return dec.Decode(dst)
}

func serviceErrorStatus(err error) int {
	switch {
	case errors.Is(err, service.ErrItemNotFound):
		return http.StatusNotFound
	case errors.Is(err, service.ErrBatchNotFound):
		return http.StatusNotFound
	case errors.Is(err, service.ErrMaterialNoItems):
		return http.StatusBadRequest
	case errors.Is(err, service.ErrItemAlreadyInBatch):
		return http.StatusConflict
	case errors.Is(err, service.ErrInvalidStep):
		return http.StatusConflict
	case errors.Is(err, service.ErrBatchClosed):
		return http.StatusConflict
	case errors.Is(err, service.ErrBatchNotRetest):
		return http.StatusConflict
	case errors.Is(err, service.ErrRetestTimeTooEarly):
		return http.StatusBadRequest
	case errors.Is(err, service.ErrItemNotInBatch):
		return http.StatusNotFound
	case errors.Is(err, service.ErrBatchCloseFailed):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func (h *Handler) handleItems(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Title    string `json:"title"`
		Material string `json:"material"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	item, err := h.svc.RegisterItem(req.Title, domain.Material(req.Material))
	if err != nil {
		writeError(w, serviceErrorStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h *Handler) handleDetections(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		ItemID     string  `json:"item_id"`
		PH         float64 `json:"ph"`
		DetectedAt string  `json:"detected_at"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	detectedAt, err := time.Parse(time.RFC3339, req.DetectedAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid time format")
		return
	}
	det, err := h.svc.AddDetection(req.ItemID, req.PH, detectedAt)
	if err != nil {
		writeError(w, serviceErrorStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, det)
}

func (h *Handler) handleBatches(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Material string `json:"material"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	batch, err := h.svc.CreateBatch(domain.Material(req.Material))
	if err != nil {
		writeError(w, serviceErrorStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, batch)
}

func (h *Handler) handleBatchByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/batches/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusBadRequest, "缺少批次ID")
		return
	}
	batchID := parts[0]
	if len(parts) == 1 {
		writeError(w, http.StatusNotFound, "未找到")
		return
	}

	switch parts[1] {
	case "steps":
		if len(parts) == 4 && parts[3] == "complete" {
			h.handleCompleteStep(w, r, batchID, parts[2])
			return
		}
	case "retests":
		h.handleRetests(w, r, batchID)
		return
	case "close":
		h.handleClose(w, r, batchID)
		return
	case "summary":
		h.handleSummary(w, r, batchID)
		return
	}
	writeError(w, http.StatusNotFound, "未找到")
}

func (h *Handler) handleCompleteStep(w http.ResponseWriter, r *http.Request, batchID, step string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	err := h.svc.CompleteStep(batchID, domain.StepType(step))
	if err != nil {
		writeError(w, serviceErrorStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handleRetests(w http.ResponseWriter, r *http.Request, batchID string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		ItemID     string  `json:"item_id"`
		PH         float64 `json:"ph"`
		RetestedAt string  `json:"retested_at"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	retestedAt, err := time.Parse(time.RFC3339, req.RetestedAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid time format")
		return
	}
	record, err := h.svc.SubmitRetest(batchID, req.ItemID, req.PH, retestedAt)
	if err != nil {
		writeError(w, serviceErrorStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, record)
}

func (h *Handler) handleClose(w http.ResponseWriter, r *http.Request, batchID string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	err := h.svc.CloseBatch(batchID)
	if err != nil {
		writeError(w, serviceErrorStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "closed"})
}

func (h *Handler) handleSummary(w http.ResponseWriter, r *http.Request, batchID string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	summary, err := h.svc.GetBatchSummary(batchID)
	if err != nil {
		writeError(w, serviceErrorStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (h *Handler) handleNeedingRetest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	items, err := h.svc.ListItemsNeedingRetest()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}
