package repository

import (
	"context"
	"nsa/internal/model"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoRepository struct {
	client *mongo.Client
	dbName string
}

// NewMongoRepository adalah constructor untuk inisialisasi koneksi ke MongoDB
func NewMongoRepository(uri string, dbName string) (*MongoRepository, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}

	// Ping database untuk memastikan koneksi benar-benar terhubung
	err = client.Ping(ctx, nil)
	if err != nil {
		return nil, err
	}

	return &MongoRepository{
		client: client,
		dbName: dbName,
	}, nil
}

// =========================================================================
// 1. MANAJEMEN SESI SLR (STATE MACHINE & HUMAN-IN-THE-LOOP)
// =========================================================================

// GetSession mengambil kondisi state riset SLR terakhir
func (r *MongoRepository) GetSession(ctx context.Context, sessionID string) (*model.SLRSession, error) {
	collection := r.client.Database(r.dbName).Collection("slr_sessions")

	var session model.SLRSession
	filter := bson.M{"_id": sessionID}

	err := collection.FindOne(ctx, filter).Decode(&session)
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// UpdateSession memperbarui kriteria, PICO, atau status alur kerja (INIT -> WAITING_APPROVAL, dll)
func (r *MongoRepository) UpdateSession(ctx context.Context, session *model.SLRSession) error {
	collection := r.client.Database(r.dbName).Collection("slr_sessions")

	filter := bson.M{"_id": session.ID}
	session.UpdatedAt = time.Now()

	update := bson.M{"$set": session}
	opts := options.Update().SetUpsert(true) // Buat baru jika ID belum ada

	_, err := collection.UpdateOne(ctx, filter, update, opts)
	return err
}

func (r *MongoRepository) GetPapersCollection() *mongo.Collection {
	return r.client.Database(r.dbName).Collection("slr_papers")
}

func (r *MongoRepository) GetPostDedupCollection() *mongo.Collection {
	return r.client.Database(r.dbName).Collection("slr_papers_post_dedup")
}

func (r *MongoRepository) ClearAndInsertPapers(ctx context.Context, sessionID string, papers []interface{}) error {
	coll := r.GetPapersCollection()
	
	// Delete old papers for this session
	_, err := coll.DeleteMany(ctx, bson.M{"session_id": sessionID})
	if err != nil {
		return err
	}
	
	// Insert new papers
	if len(papers) > 0 {
		_, err = coll.InsertMany(ctx, papers)
		return err
	}
	
	return nil
}

// GetExtractionCollection = koleksi data ekstraksi Modul 7 (satu dokumen per paper).
func (r *MongoRepository) GetExtractionCollection() *mongo.Collection {
	return r.client.Database(r.dbName).Collection("slr_extraction")
}

func (r *MongoRepository) GetScreeningCollection() *mongo.Collection {
	return r.client.Database(r.dbName).Collection("slr_screening")
}

func (r *MongoRepository) GetRandomScreeningPapers(ctx context.Context, sessionID string, limit int) ([]map[string]interface{}, error) {
	// 1. Cek apakah sudah ada batch kalibrasi yang sedang berjalan (Stateful Resume)
	filterActive := bson.M{"session_id": sessionID, "Final_Decision": "", "in_calibration_batch": true}
	findOptions := options.Find().SetLimit(int64(limit))
	cursor, err := r.GetScreeningCollection().Find(ctx, filterActive, findOptions)
	if err == nil {
		var activeResults []map[string]interface{}
		cursor.All(ctx, &activeResults)
		if len(activeResults) > 0 {
			return activeResults, nil // Resume batch sebelumnya!
		}
	}

	// 2. Jika tidak ada, ambil sample acak baru
	pipeline := mongo.Pipeline{
		{{"$match", bson.M{"session_id": sessionID, "Final_Decision": "", "in_calibration_batch": bson.M{"$ne": true}}}},
		{{"$sample", bson.M{"size": limit}}},
	}
	cursor, err = r.GetScreeningCollection().Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	var results []map[string]interface{}
	err = cursor.All(ctx, &results)
	if err != nil {
		return nil, err
	}

	// 3. Tandai (Lock) sample-sample baru ini ke dalam batch kalibrasi
	if len(results) > 0 {
		var ids []interface{}
		for _, doc := range results {
			ids = append(ids, doc["_id"])
		}
		updateFilter := bson.M{"_id": bson.M{"$in": ids}}
		updateDoc := bson.M{"$set": bson.M{"in_calibration_batch": true}}
		r.GetScreeningCollection().UpdateMany(ctx, updateFilter, updateDoc)
	}

	return results, nil
}

// =========================================================================
// 5. MANAJEMEN USER (AUTENTIKASI PASETO)
// =========================================================================

// CreateUser menyimpan pengguna baru ke dalam koleksi 'users'
func (r *MongoRepository) CreateUser(ctx context.Context, user *model.User) error {
	collection := r.client.Database(r.dbName).Collection("users")
	user.CreatedAt = time.Now()
	_, err := collection.InsertOne(ctx, user)
	return err
}

// GetUserByUsername mencari pengguna berdasarkan username
func (r *MongoRepository) GetUserByUsername(ctx context.Context, username string) (*model.User, error) {
	collection := r.client.Database(r.dbName).Collection("users")
	var user model.User
	filter := bson.M{"username": username}
	err := collection.FindOne(ctx, filter).Decode(&user)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *MongoRepository) GetUnscreenedPapers(ctx context.Context, sessionID string, limit int) ([]map[string]interface{}, error) {
	filter := bson.M{"session_id": sessionID, "Screener_1_Decision": ""}
	findOptions := options.Find().SetLimit(int64(limit))
	cursor, err := r.GetScreeningCollection().Find(ctx, filter, findOptions)
	if err != nil {
		return nil, err
	}
	var results []map[string]interface{}
	err = cursor.All(ctx, &results)
	return results, err
}

func (r *MongoRepository) GetUnevaluatedPapers(ctx context.Context, sessionID string) ([]map[string]interface{}, error) {
	filter := bson.M{
		"session_id": sessionID,
		"Screener_1_Decision": bson.M{"$ne": ""},
		"Batch_Evaluated": bson.M{"$ne": true},
	}
	cursor, err := r.GetScreeningCollection().Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	var results []map[string]interface{}
	err = cursor.All(ctx, &results)
	return results, err
}

func (r *MongoRepository) MarkPapersAsEvaluated(ctx context.Context, sessionID string, paperIDs []primitive.ObjectID) error {
	if len(paperIDs) == 0 {
		return nil
	}
	filter := bson.M{"session_id": sessionID, "_id": bson.M{"$in": paperIDs}}
	update := bson.M{"$set": bson.M{"Batch_Evaluated": true}}
	_, err := r.GetScreeningCollection().UpdateMany(ctx, filter, update)
	return err
}

// ===== Modul 6 Langkah 2: Full-text screening =====
// Paper "eligible" untuk full-text screening = INCLUDE di tahap abstrak (Modul 5)
// DAN full_text_retrieved == true (sudah tervektorisasi di Qdrant).

func fulltextEligibleFilter(sessionID string) bson.M {
	return bson.M{
		"session_id":          sessionID,
		"full_text_retrieved": true,
		"$or": []bson.M{
			{"Final_Decision": "INCLUDE"},
			{"$and": []bson.M{{"Final_Decision": ""}, {"Screener_1_Decision": "INCLUDE"}}},
		},
	}
}

func (r *MongoRepository) GetUnscreenedFullTextPapers(ctx context.Context, sessionID string, limit int) ([]map[string]interface{}, error) {
	filter := bson.M{
		"session_id":          sessionID,
		"full_text_retrieved": true,
		"$and": []bson.M{
			{"$or": []bson.M{
				{"Final_Decision": "INCLUDE"},
				{"$and": []bson.M{{"Final_Decision": ""}, {"Screener_1_Decision": "INCLUDE"}}},
			}},
			{"$or": []bson.M{
				{"Screener_1_Decision_Full": ""},
				{"Screener_1_Decision_Full": bson.M{"$exists": false}},
			}},
		},
	}
	findOptions := options.Find().SetLimit(int64(limit))
	cursor, err := r.GetScreeningCollection().Find(ctx, filter, findOptions)
	if err != nil {
		return nil, err
	}
	var results []map[string]interface{}
	err = cursor.All(ctx, &results)
	return results, err
}

func (r *MongoRepository) GetUnevaluatedFullTextPapers(ctx context.Context, sessionID string) ([]map[string]interface{}, error) {
	filter := bson.M{
		"session_id":               sessionID,
		"Screener_1_Decision_Full": bson.M{"$nin": bson.A{"", nil}},
		"Batch_Evaluated_Full":     bson.M{"$ne": true},
	}
	cursor, err := r.GetScreeningCollection().Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	var results []map[string]interface{}
	err = cursor.All(ctx, &results)
	return results, err
}

func (r *MongoRepository) MarkFullTextEvaluated(ctx context.Context, sessionID string, paperIDs []primitive.ObjectID) error {
	if len(paperIDs) == 0 {
		return nil
	}
	filter := bson.M{"session_id": sessionID, "_id": bson.M{"$in": paperIDs}}
	update := bson.M{"$set": bson.M{"Batch_Evaluated_Full": true}}
	_, err := r.GetScreeningCollection().UpdateMany(ctx, filter, update)
	return err
}

func (r *MongoRepository) GetFullTextScreeningProgress(ctx context.Context, sessionID string) (total int64, screened int64, err error) {
	coll := r.GetScreeningCollection()
	total, err = coll.CountDocuments(ctx, fulltextEligibleFilter(sessionID))
	if err != nil {
		return 0, 0, err
	}
	screenedFilter := fulltextEligibleFilter(sessionID)
	screenedFilter["Screener_1_Decision_Full"] = bson.M{"$nin": bson.A{"", nil}}
	screened, err = coll.CountDocuments(ctx, screenedFilter)
	return total, screened, err
}

// GetDisagreedFullTextPapers mengembalikan kasus yang BELUM final (Final_Decision_Full kosong)
// dan butuh keputusan manusia: DISAGREE, atau salah satu reviewer UNCERTAIN.
func (r *MongoRepository) GetDisagreedFullTextPapers(ctx context.Context, sessionID string) ([]map[string]interface{}, error) {
	filter := bson.M{
		"session_id":          sessionID,
		"Final_Decision_Full": bson.M{"$in": bson.A{"", nil}},
		"$or": []bson.M{
			{"Agreement_Full": "DISAGREE"},
			{"Screener_1_Decision_Full": "UNCERTAIN"},
			{"Screener_2_Decision_Full": "UNCERTAIN"},
		},
	}
	cursor, err := r.GetScreeningCollection().Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	var results []map[string]interface{}
	err = cursor.All(ctx, &results)
	return results, err
}

func (r *MongoRepository) UpdateScreeningPaperResolutionFull(ctx context.Context, sessionID, paperIDHex, finalDecision, notes string) error {
	objID, err := primitive.ObjectIDFromHex(paperIDHex)
	if err != nil {
		return err
	}
	filter := bson.M{"_id": objID, "session_id": sessionID}
	update := bson.M{"$set": bson.M{
		"Final_Decision_Full":      finalDecision,
		"Conflict_Resolution_Full": notes,
	}}
	_, err = r.GetScreeningCollection().UpdateOne(ctx, filter, update)
	return err
}

func (r *MongoRepository) GetScreeningProgress(ctx context.Context, sessionID string) (total int64, screened int64, err error) {
	coll := r.GetScreeningCollection()
	
	total, err = coll.CountDocuments(ctx, bson.M{"session_id": sessionID})
	if err != nil {
		return 0, 0, err
	}
	
	screened, err = coll.CountDocuments(ctx, bson.M{
		"session_id": sessionID,
		"Screener_1_Decision": bson.M{"$ne": ""},
	})
	
	return total, screened, err
}

func (r *MongoRepository) GetAllScreeningPapers(ctx context.Context, sessionID string) ([]map[string]interface{}, error) {
	cursor, err := r.GetScreeningCollection().Find(ctx, bson.M{"session_id": sessionID})
	if err != nil {
		return nil, err
	}
	var results []map[string]interface{}
	err = cursor.All(ctx, &results)
	return results, err
}

func (r *MongoRepository) UpdateScreeningPaper(ctx context.Context, id interface{}, updateDoc map[string]interface{}) error {
	filter := bson.M{"_id": id}
	_, err := r.GetScreeningCollection().UpdateOne(ctx, filter, bson.M{"$set": updateDoc})
	return err
}

func (r *MongoRepository) UpdateScreeningPaperResolution(ctx context.Context, sessionID, paperIDHex, finalDecision, notes string) error {
	objID, err := primitive.ObjectIDFromHex(paperIDHex)
	if err != nil {
		return err
	}
	filter := bson.M{"_id": objID, "session_id": sessionID}
	update := bson.M{"$set": bson.M{
		"Final_Decision": finalDecision,
		"Conflict_Resolution": notes,
	}}
	_, err = r.GetScreeningCollection().UpdateOne(ctx, filter, update)
	return err
}

// ResetCalibrationScreenings membersihkan field keputusan sebelumnya untuk persiapan re-run kalibrasi.
func (r *MongoRepository) ResetCalibrationScreenings(ctx context.Context, sessionID string) error {
	filter := bson.M{
		"session_id": sessionID,
		"Final_Decision": "", // Hanya reset yang belum di-resolve secara final
	}
	update := bson.M{
		"$set": bson.M{
			"Screener_1_Decision": "",
			"Screener_1_Reason_Code": "",
			"Screener_1_Notes": "",
			"Screener_2_Decision": "",
			"Screener_2_Reason_Code": "",
			"Screener_2_Notes": "",
			"Agreement": "",
			"Conflict_Resolution": nil,
		},
	}
	_, err := r.GetScreeningCollection().UpdateMany(ctx, filter, update)
	return err
}

// GetDisagreedPapers mengambil paper yang mengalami konflik antar reviewer (Agreement = DISAGREE)
func (r *MongoRepository) GetDisagreedPapers(ctx context.Context, sessionID string) ([]map[string]interface{}, error) {
	filter := bson.M{
		"session_id": sessionID,
		"Agreement":  "DISAGREE",
		"Final_Decision": "",
	}
	cursor, err := r.GetScreeningCollection().Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	var results []map[string]interface{}
	err = cursor.All(ctx, &results)
	return results, err
}

// =========================================================================
// 2. MANAJEMEN ARTIKEL / PAPERS (PRISMA SCREENING Pipeline)
// =========================================================================

// InsertManyPapers menyimpan ribuan metadata artikel hasil panen (Scopus/IEEE) di awal
func (r *MongoRepository) InsertManyPapers(ctx context.Context, papers []interface{}) error {
	collection := r.client.Database(r.dbName).Collection("papers")
	_, err := collection.InsertMany(ctx, papers)
	return err
}

// GetPendingPapers mengambil daftar paper yang belum diperiksa oleh Worker Agent
func (r *MongoRepository) GetPendingPapers(ctx context.Context, sessionID string) ([]model.Paper, error) {
	collection := r.client.Database(r.dbName).Collection("papers")

	filter := bson.M{
		"session_id": sessionID,
		"status":     "PENDING",
	}

	cursor, err := collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var papers []model.Paper
	if err = cursor.All(ctx, &papers); err != nil {
		return nil, err
	}
	return papers, nil
}

// UpdatePaperStatus mencatat hasil screening Worker Agent (ACCEPT/REJECT + Alasan LLM)
func (r *MongoRepository) UpdatePaperStatus(ctx context.Context, paperID string, status string, reason string) error {
	collection := r.client.Database(r.dbName).Collection("papers")

	filter := bson.M{"_id": paperID}
	update := bson.M{
		"$set": bson.M{
			"status": status,
			"reason": reason,
		},
	}

	_, err := collection.UpdateOne(ctx, filter, update)
	return err
}

// =========================================================================
// 3. MANAJEMEN KONFIGURASI PORTABEL MULTI-LLM
// =========================================================================

// GetLLMConfig mengambil API Key dan Base URL secara dinamis dari database
func (r *MongoRepository) GetLLMConfig(ctx context.Context, providerID string) (*model.LLMConfig, error) {
	collection := r.client.Database(r.dbName).Collection("llm_providers")

	var config model.LLMConfig
	filter := bson.M{"_id": providerID}

	err := collection.FindOne(ctx, filter).Decode(&config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// UpdateLLMConfig memperbarui atau membuat konfigurasi API LLM baru (Upsert)
func (r *MongoRepository) UpdateLLMConfig(ctx context.Context, config *model.LLMConfig) error {
	collection := r.client.Database(r.dbName).Collection("llm_providers")

	// Gunakan ID provider (misal: "gemini", "deepseek") sebagai key penyaring
	filter := bson.M{"_id": config.ID}
	config.UpdatedAt = time.Now()

	update := bson.M{"$set": config}
	opts := options.Update().SetUpsert(true) // Kunci otomatis membuat data baru jika belum ada

	_, err := collection.UpdateOne(ctx, filter, update, opts)
	return err
}
