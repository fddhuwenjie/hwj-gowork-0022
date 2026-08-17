package domain

import "time"

// Material 纸张材质
type Material string

const (
	MaterialXuanPaper    Material = "宣纸"
	MaterialBambooPaper  Material = "竹纸"
	MaterialMachinePaper Material = "机制纸"
)

// ItemStatus 馆藏册状态
type ItemStatus string

const (
	ItemAvailable ItemStatus = "available"
	ItemInBatch   ItemStatus = "in_batch"
	ItemCompleted ItemStatus = "completed"
)

// CollectionItem 馆藏册
type CollectionItem struct {
	ID             string     `json:"id"`
	Title          string     `json:"title"`
	Material       Material   `json:"material"`
	Status         ItemStatus `json:"status"`
	CurrentBatchID string     `json:"current_batch_id,omitempty"`
}

// Detection 酸度检测记录
type Detection struct {
	ID         string    `json:"id"`
	ItemID     string    `json:"item_id"`
	PH         float64   `json:"ph"`
	DetectedAt time.Time `json:"detected_at"`
}

// BatchStatus 批次状态
type BatchStatus string

const (
	BatchPrecheck  BatchStatus = "precheck"
	BatchDeacidify BatchStatus = "deacidify"
	BatchDrying    BatchStatus = "drying"
	BatchRetest    BatchStatus = "retest"
	BatchClosed    BatchStatus = "closed"
)

// StepType 处理步骤类型
type StepType string

const (
	StepPrecheck  StepType = "precheck"
	StepDeacidify StepType = "deacidify"
	StepDrying    StepType = "drying"
	StepRetest    StepType = "retest"
)

// StepStatus 步骤状态
type StepStatus string

const (
	StepInProgress StepStatus = "in_progress"
	StepCompleted  StepStatus = "completed"
)

// BatchStep 处理步骤
type BatchStep struct {
	Type        StepType   `json:"type"`
	Status      StepStatus `json:"status"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// TreatmentBatch 处理批次
type TreatmentBatch struct {
	ID        string      `json:"id"`
	Material  Material    `json:"material"`
	Status    BatchStatus `json:"status"`
	ItemIDs   []string    `json:"item_ids"`
	Steps     []BatchStep `json:"steps"`
	CreatedAt time.Time   `json:"created_at"`
	ClosedAt  *time.Time  `json:"closed_at,omitempty"`
}

// RetestRecord 复测记录
type RetestRecord struct {
	ID         string    `json:"id"`
	BatchID    string    `json:"batch_id"`
	ItemID     string    `json:"item_id"`
	PH         float64   `json:"ph"`
	RetestedAt time.Time `json:"retested_at"`
	Passed     bool      `json:"passed"`
}

// BatchSummary 批次摘要
type BatchSummary struct {
	BatchID      string   `json:"batch_id"`
	Material     Material `json:"material"`
	TotalItems   int      `json:"total_items"`
	PassedCount  int      `json:"passed_count"`
	FailedCount  int      `json:"failed_count"`
	PendingCount int      `json:"pending_count"`
}
