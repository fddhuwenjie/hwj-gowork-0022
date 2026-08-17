package main

import (
	"log"
	"net/http"

	"benzhi/deacidification/internal/httpapi"
	"benzhi/deacidification/internal/service"
	"benzhi/deacidification/internal/store"
)

func main() {
	memStore := store.NewMemoryStore()
	svc := service.NewBatchService(memStore)
	handler := httpapi.NewHandler(svc)

	log.Println("古籍脱酸批次服务启动于 :8080")
	log.Fatal(http.ListenAndServe(":8080", handler))
}
