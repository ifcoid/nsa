package modules
import (
	"context"
	"fmt"
	"nsa/internal/model"
)
type M3Search struct { deps *ModuleDeps }
func NewM3Search(deps *ModuleDeps) *M3Search { return &M3Search{deps: deps} }
func (m *M3Search) Name() string { return "M3_SEARCH" }
func (m *M3Search) Execute(ctx context.Context, session *model.SLRSession) error {
	fmt.Println(">> [MODUL 3: SEARCH STRATEGY] Menyusun sintaks pencarian...")
	session.Status = "M4_MINING"
	return m.deps.MongoRepo.UpdateSession(ctx, session)
}
