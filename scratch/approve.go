//go:build ignore
package main

// approve.go — menerapkan SATU persetujuan HITL untuk sesi, identik dengan
// handler produksi POST /api/sessions/{id}/approve (ApproveStep). Dijalankan
// SATU KALI setiap kali user menekan "Approve" di Telegram (keputusan manusia
// nyata). Hanya menulis satu transisi status; TIDAK menjalankan tahap apa pun.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"nsa/internal/repository"
)

const SID = "disertasi"

func main() {
	_ = godotenv.Load()
	repo, err := repository.NewMongoRepository(os.Getenv("MONGO_URI"), os.Getenv("DB_NAME"))
	if err != nil {
		fmt.Println("mongo:", err)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	s, err := repo.GetSession(ctx, SID)
	if err != nil {
		fmt.Println("getsession:", err)
		os.Exit(1)
	}
	old := s.Status

	// Transisi persis ApproveStep (session_handler.go).
	switch {
	case strings.HasSuffix(s.Status, "_WAITING_APPROVAL"):
		s.Status = s.Status[:len(s.Status)-len("_WAITING_APPROVAL")] + "_APPROVED"
	case s.Status == "M6_STEP1_WAITING_SYNC":
		s.Status = "M6_STEP2_FULLTEXT_SCREENING"
	case s.Status == "M6_STEP2_WAITING_RESOLUTION":
		s.Status = "M6_STEP2_FULLTEXT_SCREENING"
	case s.Status == "M5_STEP3_WAITING_RESOLUTION":
		s.Status = "M5_STEP3_BATCH_SCREENING"
	default:
		fmt.Printf("STATUS '%s' bukan gate yang bisa di-approve otomatis (mungkin butuh revisi/resolusi manual).\n", s.Status)
		os.Exit(3)
	}

	if e := repo.UpdateSession(ctx, s); e != nil {
		fmt.Println("update:", e)
		os.Exit(1)
	}
	fmt.Printf("APPROVED: %s -> %s\n", old, s.Status)
}
