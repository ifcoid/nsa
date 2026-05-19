package modules
import (
	"context"
	"fmt"
	"nsa/internal/model"
)
type M8Synthesis struct { deps *ModuleDeps }
func NewM8Synthesis(deps *ModuleDeps) *M8Synthesis { return &M8Synthesis{deps: deps} }
func (m *M8Synthesis) Name() string { return "M8_SYNTHESIS" }
func (m *M8Synthesis) Execute(ctx context.Context, session *model.SLRSession) error {
	fmt.Println(">> [MODUL 8: SYNTHESIS] Analisis deskriptif, path execution (A/B), dan evidence grading...")
	session.Status = "M9_MANUSCRIPT"
	return m.deps.MongoRepo.UpdateSession(ctx, session)
}
