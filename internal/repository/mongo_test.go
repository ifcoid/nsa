package repository

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"

	"nsa/internal/model"
)

// TestSaveSessionUnsetting_SendsUnset is the regression guard for the omitempty+$set
// gotcha: it asserts that clearing a field actually emits a real Mongo $unset (not a
// silently-dropped nil in $set). If anyone reverts SaveSessionUnsetting to a plain
// UpdateSession, this test fails.
func TestSaveSessionUnsetting_SendsUnset(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))

	mt.Run("clears pico_audit_log via real $unset", func(mt *mtest.T) {
		mt.AddMockResponses(mtest.CreateSuccessResponse(
			bson.E{Key: "n", Value: 1}, bson.E{Key: "nModified", Value: 1},
		))
		repo := &MongoRepository{client: mt.Client, dbName: "testdb"}

		if err := repo.SaveSessionUnsetting(context.Background(), &model.SLRSession{ID: "s1"}, "pico_audit_log"); err != nil {
			t.Fatalf("SaveSessionUnsetting error: %v", err)
		}

		evt := mt.GetStartedEvent()
		if evt == nil || evt.CommandName != "update" {
			t.Fatalf("expected an 'update' command, got %+v", evt)
		}
		u := firstUpdate(t, evt.Command)

		unsetVal, err := u.LookupErr("$unset")
		if err != nil {
			t.Fatalf("expected $unset in update, got: %s", u.String())
		}
		if _, err := unsetVal.Document().LookupErr("pico_audit_log"); err != nil {
			t.Errorf("expected $unset.pico_audit_log, got: %s", unsetVal.Document().String())
		}
		// $set (full struct save) must still be present alongside $unset.
		if _, err := u.LookupErr("$set"); err != nil {
			t.Errorf("expected $set alongside $unset")
		}
	})

	mt.Run("clears manuscript via real $unset", func(mt *mtest.T) {
		mt.AddMockResponses(mtest.CreateSuccessResponse(
			bson.E{Key: "n", Value: 1}, bson.E{Key: "nModified", Value: 1},
		))
		repo := &MongoRepository{client: mt.Client, dbName: "testdb"}
		if err := repo.SaveSessionUnsetting(context.Background(), &model.SLRSession{ID: "s2"}, "manuscript"); err != nil {
			t.Fatalf("error: %v", err)
		}
		u := firstUpdate(t, mt.GetStartedEvent().Command)
		unsetVal, err := u.LookupErr("$unset")
		if err != nil {
			t.Fatalf("expected $unset, got: %s", u.String())
		}
		if _, err := unsetVal.Document().LookupErr("manuscript"); err != nil {
			t.Errorf("expected $unset.manuscript, got: %s", unsetVal.Document().String())
		}
	})

	mt.Run("no unset fields -> no $unset key", func(mt *mtest.T) {
		mt.AddMockResponses(mtest.CreateSuccessResponse(
			bson.E{Key: "n", Value: 1}, bson.E{Key: "nModified", Value: 1},
		))
		repo := &MongoRepository{client: mt.Client, dbName: "testdb"}
		if err := repo.SaveSessionUnsetting(context.Background(), &model.SLRSession{ID: "s3"}); err != nil {
			t.Fatalf("error: %v", err)
		}
		u := firstUpdate(t, mt.GetStartedEvent().Command)
		if _, err := u.LookupErr("$unset"); err == nil {
			t.Errorf("did not expect $unset when no fields given: %s", u.String())
		}
	})
}

// firstUpdate extracts the update document ({$set, $unset, ...}) of the first statement
// in a captured Mongo `update` command.
func firstUpdate(t *testing.T, cmd bson.Raw) bson.Raw {
	t.Helper()
	updatesVal, err := cmd.LookupErr("updates")
	if err != nil {
		t.Fatalf("no 'updates' in command: %s", cmd.String())
	}
	elems, err := updatesVal.Array().Elements()
	if err != nil || len(elems) == 0 {
		t.Fatalf("'updates' is not a non-empty array: %v", err)
	}
	uVal, err := elems[0].Value().Document().LookupErr("u")
	if err != nil {
		t.Fatalf("no 'u' in first update statement: %v", err)
	}
	return uVal.Document()
}
