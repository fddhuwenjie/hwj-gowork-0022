package service

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"benzhi/deacidification/internal/domain"
	"benzhi/deacidification/internal/store"
)

const pHThreshold = 7.0

var (
	ErrItemNotFound       = store.ErrItemNotFound
	ErrBatchNotFound      = store.ErrBatchNotFound
	ErrMaterialNoItems    = errors.New("该材质没有可用馆藏册")
	ErrItemAlreadyInBatch = errors.New("馆藏册已在其他未结束批次中")
	ErrInvalidStep        = errors.New("无效的步骤或步骤顺序错误")
	ErrBatchClosed        = errors.New("批次已关闭")
	ErrBatchNotRetest     = errors.New("批次不在复测阶段")
	ErrRetestTimeTooEarly = errors.New("复测时间必须晚于干燥完成时间")
	ErrItemNotInBatch     = errors.New("馆藏册不属于该批次")
	ErrBatchCloseFailed   = errors.New("批次关闭失败：存在未合格复测记录")
)

// BatchService 脱酸批次业务服务
type BatchService struct {
	store *store.MemoryStore
	mu    sync.Mutex
	idSeq uint64
}

func NewBatchService(s *store.MemoryStore) *BatchService {
	return &BatchService{store: s}
}

func (s *BatchService) nextID(prefix string) string {
	n := atomic.AddUint64(&s.idSeq, 1)
	return fmt.Sprintf("%s-%d", prefix, n)
}

// RegisterItem 登记馆藏册
func (s *BatchService) RegisterItem(title string, material domain.Material) (*domain.CollectionItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := &domain.CollectionItem{
		ID:       s.nextID("item"),
		Title:    title,
		Material: material,
		Status:   domain.ItemAvailable,
	}
	if err := s.store.SaveItem(item); err != nil {
		return nil, err
	}
	return item, nil
}

// AddDetection 记录酸度检测
func (s *BatchService) AddDetection(itemID string, ph float64, detectedAt time.Time) (*domain.Detection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.store.GetItem(itemID); err != nil {
		return nil, err
	}
	det := &domain.Detection{
		ID:         s.nextID("det"),
		ItemID:     itemID,
		PH:         ph,
		DetectedAt: detectedAt,
	}
	if err := s.store.SaveDetection(det); err != nil {
		return nil, err
	}
	return det, nil
}

// CreateBatch 按材质创建批次，自动收集该材质下可用馆藏册
func (s *BatchService) CreateBatch(material domain.Material) (*domain.TreatmentBatch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items, err := s.store.ListItems()
	if err != nil {
		return nil, err
	}

	var itemIDs []string
	for _, item := range items {
		if item.Material != material {
			continue
		}
		switch item.Status {
		case domain.ItemAvailable:
			itemIDs = append(itemIDs, item.ID)
		}
	}
	if len(itemIDs) == 0 {
		return nil, ErrMaterialNoItems
	}

	batchID := s.nextID("batch")
	now := time.Now()
	batch := &domain.TreatmentBatch{
		ID:        batchID,
		Material:  material,
		Status:    domain.BatchPrecheck,
		ItemIDs:   itemIDs,
		CreatedAt: now,
		Steps: []domain.BatchStep{
			{Type: domain.StepPrecheck, Status: domain.StepInProgress, StartedAt: now},
		},
	}

	// 更新馆藏册状态
	for _, id := range itemIDs {
		item, err := s.store.GetItem(id)
		if err != nil {
			return nil, err
		}
		item.Status = domain.ItemInBatch
		item.CurrentBatchID = batchID
		if err := s.store.UpdateItem(item); err != nil {
			return nil, err
		}
	}

	if err := s.store.SaveBatch(batch); err != nil {
		return nil, err
	}
	return batch, nil
}

func latestRetestsByItem(records []*domain.RetestRecord) map[string]*domain.RetestRecord {
	latest := make(map[string]*domain.RetestRecord)
	for _, record := range records {
		current, exists := latest[record.ItemID]
		if !exists || !record.RetestedAt.Before(current.RetestedAt) {
			latest[record.ItemID] = record
		}
	}
	return latest
}

// CompleteStep 完成当前步骤并推进到下一步
func (s *BatchService) CompleteStep(batchID string, step domain.StepType) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	batch, err := s.store.GetBatch(batchID)
	if err != nil {
		return err
	}
	if batch.Status == domain.BatchClosed {
		return ErrBatchClosed
	}

	// 校验步骤与批次当前状态匹配
	var currentStep domain.StepType
	switch batch.Status {
	case domain.BatchPrecheck:
		currentStep = domain.StepPrecheck
	case domain.BatchDeacidify:
		currentStep = domain.StepDeacidify
	case domain.BatchDrying:
		currentStep = domain.StepDrying
	case domain.BatchRetest:
		currentStep = domain.StepRetest
	default:
		return ErrInvalidStep
	}
	if currentStep != step {
		return ErrInvalidStep
	}
	if step == domain.StepRetest {
		return ErrInvalidStep // 复测需通过提交记录和关闭批次完成
	}

	// 标记当前步骤完成
	now := time.Now()
	steps := batch.Steps
	for i := range steps {
		if steps[i].Type == step && steps[i].Status == domain.StepInProgress {
			steps[i].Status = domain.StepCompleted
			steps[i].CompletedAt = &now
			break
		}
	}

	// 添加下一步骤
	var nextStepType domain.StepType
	var nextBatchStatus domain.BatchStatus
	switch step {
	case domain.StepPrecheck:
		nextStepType = domain.StepDeacidify
		nextBatchStatus = domain.BatchDeacidify
	case domain.StepDeacidify:
		nextStepType = domain.StepDrying
		nextBatchStatus = domain.BatchDrying
	case domain.StepDrying:
		nextStepType = domain.StepRetest
		nextBatchStatus = domain.BatchRetest
	}
	steps = append(steps, domain.BatchStep{
		Type:      nextStepType,
		Status:    domain.StepInProgress,
		StartedAt: now,
	})

	batch.Steps = steps
	batch.Status = nextBatchStatus
	return s.store.UpdateBatch(batch)
}

// SubmitRetest 提交复测记录
func (s *BatchService) SubmitRetest(batchID, itemID string, ph float64, retestedAt time.Time) (*domain.RetestRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	batch, err := s.store.GetBatch(batchID)
	if err != nil {
		return nil, err
	}
	if batch.Status != domain.BatchRetest {
		return nil, ErrBatchNotRetest
	}

	// 检查馆藏册属于该批次
	inBatch := false
	for _, id := range batch.ItemIDs {
		if id == itemID {
			inBatch = true
			break
		}
	}
	if !inBatch {
		return nil, ErrItemNotInBatch
	}

	// 获取干燥步骤完成时间
	var dryingCompleted *time.Time
	for _, step := range batch.Steps {
		if step.Type == domain.StepDrying && step.Status == domain.StepCompleted {
			dryingCompleted = step.CompletedAt
			break
		}
	}
	if dryingCompleted == nil {
		return nil, ErrRetestTimeTooEarly
	}
	if !retestedAt.After(*dryingCompleted) {
		return nil, ErrRetestTimeTooEarly
	}

	passed := ph >= pHThreshold
	record := &domain.RetestRecord{
		ID:         s.nextID("retest"),
		BatchID:    batchID,
		ItemID:     itemID,
		PH:         ph,
		RetestedAt: retestedAt,
		Passed:     passed,
	}
	if err := s.store.SaveRetestRecord(record); err != nil {
		return nil, err
	}
	return record, nil
}

// CloseBatch 关闭批次，要求所有馆藏册复测合格
func (s *BatchService) CloseBatch(batchID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	batch, err := s.store.GetBatch(batchID)
	if err != nil {
		return err
	}
	if batch.Status != domain.BatchRetest {
		return ErrBatchNotRetest
	}

	retests, err := s.store.ListRetestRecords(batchID)
	if err != nil {
		return err
	}
	latest := latestRetestsByItem(retests)
	for _, id := range batch.ItemIDs {
		if latest[id] == nil || !latest[id].Passed {
			return ErrBatchCloseFailed
		}
	}

	// 全部合格，更新馆藏册状态
	for _, id := range batch.ItemIDs {
		item, err := s.store.GetItem(id)
		if err != nil {
			return err
		}
		item.Status = domain.ItemCompleted
		item.CurrentBatchID = ""
		if err := s.store.UpdateItem(item); err != nil {
			return err
		}
	}

	now := time.Now()
	batch.Status = domain.BatchClosed
	batch.ClosedAt = &now
	return s.store.UpdateBatch(batch)
}

// ListItemsNeedingRetest 返回所有在未关闭批次中且没有合格复测记录的馆藏册
func (s *BatchService) ListItemsNeedingRetest() ([]*domain.CollectionItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	batches, err := s.store.ListBatches()
	if err != nil {
		return nil, err
	}
	var result []*domain.CollectionItem
	seen := make(map[string]bool)
	for _, batch := range batches {
		if batch.Status == domain.BatchClosed {
			continue
		}
		retests, err := s.store.ListRetestRecords(batch.ID)
		if err != nil {
			return nil, err
		}
		latest := latestRetestsByItem(retests)
		for _, itemID := range batch.ItemIDs {
			if (latest[itemID] == nil || !latest[itemID].Passed) && !seen[itemID] {
				item, err := s.store.GetItem(itemID)
				if err != nil {
					return nil, err
				}
				result = append(result, item)
				seen[itemID] = true
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

// GetBatchSummary 获取批次摘要，按复测结论汇总
func (s *BatchService) GetBatchSummary(batchID string) (*domain.BatchSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	batch, err := s.store.GetBatch(batchID)
	if err != nil {
		return nil, err
	}

	retests, err := s.store.ListRetestRecords(batchID)
	if err != nil {
		return nil, err
	}

	latest := latestRetestsByItem(retests)

	total := len(batch.ItemIDs)
	passed := 0
	failed := 0
	for _, id := range batch.ItemIDs {
		if latest[id] != nil && latest[id].Passed {
			passed++
		} else if latest[id] != nil {
			failed++
		}
	}
	pending := total - passed - failed

	return &domain.BatchSummary{
		BatchID:      batch.ID,
		Material:     batch.Material,
		TotalItems:   total,
		PassedCount:  passed,
		FailedCount:  failed,
		PendingCount: pending,
	}, nil
}
