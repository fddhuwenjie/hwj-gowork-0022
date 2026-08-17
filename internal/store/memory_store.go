package store

import (
	"errors"
	"sync"

	"benzhi/deacidification/internal/domain"
)

var (
	ErrItemNotFound    = errors.New("馆藏册不存在")
	ErrBatchNotFound   = errors.New("批次不存在")
	ErrDetectionExists = errors.New("检测记录已存在")
	ErrBatchExists     = errors.New("批次已存在")
)

// MemoryStore 线程安全内存存储
type MemoryStore struct {
	mu         sync.RWMutex
	items      map[string]*domain.CollectionItem
	detections map[string]*domain.Detection
	batches    map[string]*domain.TreatmentBatch
	retests    map[string][]*domain.RetestRecord // key: batchID
}

func cloneItem(item *domain.CollectionItem) *domain.CollectionItem {
	if item == nil {
		return nil
	}
	cloned := *item
	return &cloned
}

func cloneDetection(detection *domain.Detection) *domain.Detection {
	if detection == nil {
		return nil
	}
	cloned := *detection
	return &cloned
}

func cloneBatch(batch *domain.TreatmentBatch) *domain.TreatmentBatch {
	if batch == nil {
		return nil
	}
	cloned := *batch
	cloned.ItemIDs = append([]string(nil), batch.ItemIDs...)
	cloned.Steps = append([]domain.BatchStep(nil), batch.Steps...)
	for i := range cloned.Steps {
		if batch.Steps[i].CompletedAt != nil {
			completedAt := *batch.Steps[i].CompletedAt
			cloned.Steps[i].CompletedAt = &completedAt
		}
	}
	if batch.ClosedAt != nil {
		closedAt := *batch.ClosedAt
		cloned.ClosedAt = &closedAt
	}
	return &cloned
}

func cloneRetest(record *domain.RetestRecord) *domain.RetestRecord {
	if record == nil {
		return nil
	}
	cloned := *record
	return &cloned
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		items:      make(map[string]*domain.CollectionItem),
		detections: make(map[string]*domain.Detection),
		batches:    make(map[string]*domain.TreatmentBatch),
		retests:    make(map[string][]*domain.RetestRecord),
	}
}

func (s *MemoryStore) SaveItem(item *domain.CollectionItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.items[item.ID]; exists {
		return errors.New("馆藏册已存在")
	}
	s.items[item.ID] = cloneItem(item)
	return nil
}

func (s *MemoryStore) UpdateItem(item *domain.CollectionItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.items[item.ID]; !exists {
		return ErrItemNotFound
	}
	s.items[item.ID] = cloneItem(item)
	return nil
}

func (s *MemoryStore) GetItem(id string) (*domain.CollectionItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[id]
	if !ok {
		return nil, ErrItemNotFound
	}
	return cloneItem(item), nil
}

func (s *MemoryStore) ListItems() ([]*domain.CollectionItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]*domain.CollectionItem, 0, len(s.items))
	for _, item := range s.items {
		items = append(items, cloneItem(item))
	}
	return items, nil
}

func (s *MemoryStore) SaveDetection(det *domain.Detection) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.detections[det.ID]; exists {
		return ErrDetectionExists
	}
	s.detections[det.ID] = cloneDetection(det)
	return nil
}

func (s *MemoryStore) SaveBatch(batch *domain.TreatmentBatch) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.batches[batch.ID]; exists {
		return ErrBatchExists
	}
	s.batches[batch.ID] = cloneBatch(batch)
	return nil
}

func (s *MemoryStore) UpdateBatch(batch *domain.TreatmentBatch) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.batches[batch.ID]; !exists {
		return ErrBatchNotFound
	}
	s.batches[batch.ID] = cloneBatch(batch)
	return nil
}

func (s *MemoryStore) GetBatch(id string) (*domain.TreatmentBatch, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	batch, ok := s.batches[id]
	if !ok {
		return nil, ErrBatchNotFound
	}
	return cloneBatch(batch), nil
}

func (s *MemoryStore) ListBatches() ([]*domain.TreatmentBatch, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	batches := make([]*domain.TreatmentBatch, 0, len(s.batches))
	for _, batch := range s.batches {
		batches = append(batches, cloneBatch(batch))
	}
	return batches, nil
}

func (s *MemoryStore) SaveRetestRecord(record *domain.RetestRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.retests[record.BatchID] = append(s.retests[record.BatchID], cloneRetest(record))
	return nil
}

func (s *MemoryStore) ListRetestRecords(batchID string) ([]*domain.RetestRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	records, ok := s.retests[batchID]
	if !ok {
		return []*domain.RetestRecord{}, nil
	}
	copied := make([]*domain.RetestRecord, len(records))
	for i, record := range records {
		copied[i] = cloneRetest(record)
	}
	return copied, nil
}
