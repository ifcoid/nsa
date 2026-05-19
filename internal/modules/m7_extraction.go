package modules
import (
	"context"
	"fmt"
	"nsa/internal/model"
)
type M7Extraction struct { deps *ModuleDeps }
func NewM7Extraction(deps *ModuleDeps) *M7Extraction { return &M7Extraction{deps: deps} }
func (m *M7Extraction) Name() string { return "M7_EXTRACTION" }
func (m *M7Extraction) Execute(ctx context.Context, session *model.SLRSession) error {
	fmt.Println(">> [MODUL 7: EXTRACTION] Mengekstrak data ke framework terstruktur (QA)...")
	session.Status = "M8_SYNTHESIS"
	return m.deps.MongoRepo.UpdateSession(ctx, session)
}
