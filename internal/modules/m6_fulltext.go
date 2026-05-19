package modules
import (
	"context"
	"fmt"
	"nsa/internal/model"
)
type M6Fulltext struct { deps *ModuleDeps }
func NewM6Fulltext(deps *ModuleDeps) *M6Fulltext { return &M6Fulltext{deps: deps} }
func (m *M6Fulltext) Name() string { return "M6_FULLTEXT" }
func (m *M6Fulltext) Execute(ctx context.Context, session *model.SLRSession) error {
	fmt.Println(">> [MODUL 6: FULLTEXT] Mengunduh PDF dan menyaring eligibility mendalam...")
	session.Status = "M7_EXTRACTION"
	return m.deps.MongoRepo.UpdateSession(ctx, session)
}
