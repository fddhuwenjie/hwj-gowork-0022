package service

import (
	"testing"
	"time"

	"benzhi/deacidification/internal/domain"
	"benzhi/deacidification/internal/store"
)

func setupService() *BatchService {
	memStore := store.NewMemoryStore()
	return NewBatchService(memStore)
}

func registerItems(t *testing.T, svc *BatchService, titles []string, material domain.Material) []*domain.CollectionItem {
	t.Helper()
	items := make([]*domain.CollectionItem, 0, len(titles))
	for _, title := range titles {
		item, err := svc.RegisterItem(title, material)
		if err != nil {
			t.Fatalf("register item: %v", err)
		}
		items = append(items, item)
	}
	return items
}

func completeStepsUntilRetest(t *testing.T, svc *BatchService, batchID string) *domain.TreatmentBatch {
	t.Helper()
	if err := svc.CompleteStep(batchID, domain.StepPrecheck); err != nil {
		t.Fatalf("complete precheck: %v", err)
	}
	if err := svc.CompleteStep(batchID, domain.StepDeacidify); err != nil {
		t.Fatalf("complete deacidify: %v", err)
	}
	if err := svc.CompleteStep(batchID, domain.StepDrying); err != nil {
		t.Fatalf("complete drying: %v", err)
	}
	batch, err := svc.store.GetBatch(batchID)
	if err != nil {
		t.Fatalf("get batch after steps: %v", err)
	}
	if batch.Status != domain.BatchRetest {
		t.Fatalf("expected retest status, got %s", batch.Status)
	}
	return batch
}

func TestFullBusinessChain(t *testing.T) {
	svc := setupService()
	items := registerItems(t, svc, []string{"古籍一", "古籍二"}, domain.MaterialXuanPaper)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	batch, err := svc.CreateBatch(domain.MaterialXuanPaper)
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	if batch.Status != domain.BatchPrecheck {
		t.Fatalf("expected precheck, got %s", batch.Status)
	}
	if len(batch.ItemIDs) != 2 {
		t.Fatalf("expected 2 items in batch, got %d", len(batch.ItemIDs))
	}

	// 校验册状态
	for _, id := range batch.ItemIDs {
		item, _ := svc.store.GetItem(id)
		if item.Status != domain.ItemInBatch {
			t.Fatalf("item %s status not in_batch", id)
		}
		if item.CurrentBatchID != batch.ID {
			t.Fatalf("item %s current batch id mismatch", id)
		}
	}

	// 完成预检、脱酸、干燥
	batch = completeStepsUntilRetest(t, svc, batch.ID)

	// 获取干燥完成时间
	var dryingCompleted *time.Time
	for _, step := range batch.Steps {
		if step.Type == domain.StepDrying && step.Status == domain.StepCompleted {
			dryingCompleted = step.CompletedAt
			break
		}
	}
	if dryingCompleted == nil {
		t.Fatal("drying step not completed")
	}
	retestTime := dryingCompleted.Add(time.Second)

	// 提交复测，全部合格
	for _, id := range batch.ItemIDs {
		if _, err := svc.SubmitRetest(batch.ID, id, 7.5, retestTime); err != nil {
			t.Fatalf("submit retest for %s: %v", id, err)
		}
	}

	// 关闭批次
	if err := svc.CloseBatch(batch.ID); err != nil {
		t.Fatalf("close batch: %v", err)
	}

	// 验证批次关闭
	batch, _ = svc.store.GetBatch(batch.ID)
	if batch.Status != domain.BatchClosed {
		t.Fatalf("expected closed, got %s", batch.Status)
	}
	if batch.ClosedAt == nil {
		t.Fatal("closed time should be set")
	}

	// 验证册状态
	for _, id := range batch.ItemIDs {
		item, _ := svc.store.GetItem(id)
		if item.Status != domain.ItemCompleted {
			t.Fatalf("item %s not completed", id)
		}
		if item.CurrentBatchID != "" {
			t.Fatalf("item %s current batch id not cleared", id)
		}
	}

	// 摘要
	summary, err := svc.GetBatchSummary(batch.ID)
	if err != nil {
		t.Fatalf("get summary: %v", err)
	}
	if summary.TotalItems != 2 || summary.PassedCount != 2 || summary.FailedCount != 0 || summary.PendingCount != 0 {
		t.Fatalf("summary mismatch: %+v", summary)
	}

	// 查询需复测：应为空
	needing, err := svc.ListItemsNeedingRetest()
	if err != nil {
		t.Fatalf("list needing retest: %v", err)
	}
	if len(needing) != 0 {
		t.Fatalf("expected no needing retest, got %d", len(needing))
	}
}

func TestStepOrderViolation(t *testing.T) {
	svc := setupService()
	registerItems(t, svc, []string{"古籍一"}, domain.MaterialXuanPaper)
	batch, err := svc.CreateBatch(domain.MaterialXuanPaper)
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}

	// 跳过预检，尝试完成脱酸
	err = svc.CompleteStep(batch.ID, domain.StepDeacidify)
	if err == nil {
		t.Fatal("expected error completing deacidify before precheck")
	}

	// 状态应保持不变
	b, _ := svc.store.GetBatch(batch.ID)
	if b.Status != domain.BatchPrecheck {
		t.Fatalf("batch status changed, got %s", b.Status)
	}
	for _, id := range b.ItemIDs {
		item, _ := svc.store.GetItem(id)
		if item.Status != domain.ItemInBatch {
			t.Fatalf("item status changed")
		}
	}
}

func TestNewItemCanEnterAnotherActiveBatchWithoutReusingOccupiedItem(t *testing.T) {
	svc := setupService()
	items := registerItems(t, svc, []string{"古籍一"}, domain.MaterialXuanPaper)
	first, err := svc.CreateBatch(domain.MaterialXuanPaper)
	if err != nil {
		t.Fatalf("first batch create: %v", err)
	}

	newItem := registerItems(t, svc, []string{"古籍二"}, domain.MaterialXuanPaper)[0]
	second, err := svc.CreateBatch(domain.MaterialXuanPaper)
	if err != nil {
		t.Fatalf("second batch create: %v", err)
	}
	if len(second.ItemIDs) != 1 || second.ItemIDs[0] != newItem.ID {
		t.Fatalf("second batch reused occupied items: %+v", second.ItemIDs)
	}
	occupied, _ := svc.store.GetItem(items[0].ID)
	if occupied.CurrentBatchID != first.ID {
		t.Fatalf("first item moved from %s to %s", first.ID, occupied.CurrentBatchID)
	}
}

func TestLatestRetestControlsCloseSummaryAndPendingQuery(t *testing.T) {
	svc := setupService()
	item := registerItems(t, svc, []string{"古籍一"}, domain.MaterialXuanPaper)[0]
	batch, err := svc.CreateBatch(domain.MaterialXuanPaper)
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	batch = completeStepsUntilRetest(t, svc, batch.ID)
	dryingCompleted := batch.Steps[len(batch.Steps)-2].CompletedAt
	if _, err := svc.SubmitRetest(batch.ID, item.ID, 7.5, dryingCompleted.Add(time.Second)); err != nil {
		t.Fatalf("submit passing retest: %v", err)
	}
	if _, err := svc.SubmitRetest(batch.ID, item.ID, 6.5, dryingCompleted.Add(2*time.Second)); err != nil {
		t.Fatalf("submit later failed retest: %v", err)
	}

	if err := svc.CloseBatch(batch.ID); err != ErrBatchCloseFailed {
		t.Fatalf("expected close failure from latest retest, got %v", err)
	}
	summary, err := svc.GetBatchSummary(batch.ID)
	if err != nil {
		t.Fatalf("get summary: %v", err)
	}
	if summary.PassedCount != 0 || summary.FailedCount != 1 || summary.PendingCount != 0 {
		t.Fatalf("summary did not use latest retest: %+v", summary)
	}
	pending, err := svc.ListItemsNeedingRetest()
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != item.ID {
		t.Fatalf("latest failed item missing from pending list: %+v", pending)
	}
}

func TestNeedingRetestOrderIsStable(t *testing.T) {
	svc := setupService()
	items := registerItems(t, svc, []string{"古籍一", "古籍二", "古籍三"}, domain.MaterialXuanPaper)
	if _, err := svc.CreateBatch(domain.MaterialXuanPaper); err != nil {
		t.Fatalf("create batch: %v", err)
	}
	for run := 0; run < 20; run++ {
		pending, err := svc.ListItemsNeedingRetest()
		if err != nil {
			t.Fatalf("list pending: %v", err)
		}
		for i := range items {
			if pending[i].ID != items[i].ID {
				t.Fatalf("unstable order on run %d: %+v", run, pending)
			}
		}
	}
}

func TestNeedingRetestKeepsFailedAndPendingItemsWhenSiblingPassed(t *testing.T) {
	svc := setupService()
	items := registerItems(t, svc, []string{"古籍一", "古籍二", "古籍三"}, domain.MaterialXuanPaper)
	batch, err := svc.CreateBatch(domain.MaterialXuanPaper)
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	batch = completeStepsUntilRetest(t, svc, batch.ID)
	dryingCompleted := batch.Steps[len(batch.Steps)-2].CompletedAt
	if _, err := svc.SubmitRetest(batch.ID, items[0].ID, 7.5, dryingCompleted.Add(time.Second)); err != nil {
		t.Fatalf("submit passing retest: %v", err)
	}
	if _, err := svc.SubmitRetest(batch.ID, items[1].ID, 6.5, dryingCompleted.Add(time.Second)); err != nil {
		t.Fatalf("submit failed retest: %v", err)
	}

	pending, err := svc.ListItemsNeedingRetest()
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(pending) != 2 || pending[0].ID != items[1].ID || pending[1].ID != items[2].ID {
		t.Fatalf("expected failed and untested items, got %+v", pending)
	}
}

func TestRetestTimeConstraint(t *testing.T) {
	svc := setupService()
	registerItems(t, svc, []string{"古籍一"}, domain.MaterialXuanPaper)
	batch, err := svc.CreateBatch(domain.MaterialXuanPaper)
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	batch = completeStepsUntilRetest(t, svc, batch.ID)

	// 获取干燥完成时间
	var dryingCompleted *time.Time
	for _, step := range batch.Steps {
		if step.Type == domain.StepDrying && step.Status == domain.StepCompleted {
			dryingCompleted = step.CompletedAt
			break
		}
	}
	// 提交早于干燥完成的时间
	earlyTime := dryingCompleted.Add(-time.Second)
	_, err = svc.SubmitRetest(batch.ID, batch.ItemIDs[0], 7.5, earlyTime)
	if err == nil {
		t.Fatal("expected error for early retest")
	}
	if err != ErrRetestTimeTooEarly {
		t.Fatalf("expected ErrRetestTimeTooEarly, got %v", err)
	}

	// 不应保存记录
	records, _ := svc.store.ListRetestRecords(batch.ID)
	if len(records) != 0 {
		t.Fatalf("expected no records, got %d", len(records))
	}
}

func TestCloseBatchFailureKeepsState(t *testing.T) {
	svc := setupService()
	registerItems(t, svc, []string{"古籍一", "古籍二"}, domain.MaterialXuanPaper)
	batch, err := svc.CreateBatch(domain.MaterialXuanPaper)
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	batch = completeStepsUntilRetest(t, svc, batch.ID)

	// 只提交一个合格
	var dryingCompleted *time.Time
	for _, step := range batch.Steps {
		if step.Type == domain.StepDrying && step.Status == domain.StepCompleted {
			dryingCompleted = step.CompletedAt
			break
		}
	}
	retestTime := dryingCompleted.Add(time.Second)
	_, err = svc.SubmitRetest(batch.ID, batch.ItemIDs[0], 7.5, retestTime)
	if err != nil {
		t.Fatalf("submit retest for first item: %v", err)
	}

	// 尝试关闭，应失败
	err = svc.CloseBatch(batch.ID)
	if err == nil {
		t.Fatal("expected close failure")
	}

	// 验证状态不变
	b, _ := svc.store.GetBatch(batch.ID)
	if b.Status != domain.BatchRetest {
		t.Fatalf("batch status changed, got %s", b.Status)
	}
	if b.ClosedAt != nil {
		t.Fatal("closed time should be nil")
	}
	for _, id := range batch.ItemIDs {
		item, _ := svc.store.GetItem(id)
		if item.Status != domain.ItemInBatch {
			t.Fatalf("item %s status changed", id)
		}
		if item.CurrentBatchID != batch.ID {
			t.Fatalf("item %s current batch id changed", id)
		}
	}
}

func TestBatchSummaryWithMixedRetestResults(t *testing.T) {
	svc := setupService()
	registerItems(t, svc, []string{"古籍一", "古籍二", "古籍三"}, domain.MaterialXuanPaper)
	batch, err := svc.CreateBatch(domain.MaterialXuanPaper)
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	batch = completeStepsUntilRetest(t, svc, batch.ID)

	var dryingCompleted *time.Time
	for _, step := range batch.Steps {
		if step.Type == domain.StepDrying && step.Status == domain.StepCompleted {
			dryingCompleted = step.CompletedAt
			break
		}
	}
	retestTime := dryingCompleted.Add(time.Second)

	// 第一个合格
	if _, err := svc.SubmitRetest(batch.ID, batch.ItemIDs[0], 7.5, retestTime); err != nil {
		t.Fatalf("submit retest 0: %v", err)
	}
	// 第二个不合格
	if _, err := svc.SubmitRetest(batch.ID, batch.ItemIDs[1], 6.5, retestTime); err != nil {
		t.Fatalf("submit retest 1: %v", err)
	}
	// 第三个未提交

	summary, err := svc.GetBatchSummary(batch.ID)
	if err != nil {
		t.Fatalf("get summary: %v", err)
	}
	if summary.TotalItems != 3 {
		t.Fatalf("expected total 3, got %d", summary.TotalItems)
	}
	if summary.PassedCount != 1 {
		t.Fatalf("expected passed 1, got %d", summary.PassedCount)
	}
	if summary.FailedCount != 1 {
		t.Fatalf("expected failed 1, got %d", summary.FailedCount)
	}
	if summary.PendingCount != 1 {
		t.Fatalf("expected pending 1, got %d", summary.PendingCount)
	}
}
