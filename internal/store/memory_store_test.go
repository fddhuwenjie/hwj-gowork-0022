package store

import (
	"testing"

	"benzhi/deacidification/internal/domain"
)

func TestStoredItemsAreIsolatedFromCallerMutation(t *testing.T) {
	store := NewMemoryStore()
	item := &domain.CollectionItem{ID: "item-1", Title: "原题名", Status: domain.ItemAvailable}
	if err := store.SaveItem(item); err != nil {
		t.Fatalf("save item: %v", err)
	}
	item.Title = "外部改写"

	first, err := store.GetItem(item.ID)
	if err != nil {
		t.Fatalf("get item: %v", err)
	}
	if first.Title != "原题名" {
		t.Fatalf("save retained caller pointer: %q", first.Title)
	}
	first.Title = "再次改写"
	second, err := store.GetItem(item.ID)
	if err != nil {
		t.Fatalf("get item again: %v", err)
	}
	if second.Title != "原题名" {
		t.Fatalf("get exposed stored pointer: %q", second.Title)
	}
}
