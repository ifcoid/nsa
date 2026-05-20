package repository

import (
	"context"
	"nsa/internal/model"
	"time"

	"go.mongodb.org/mongo-driver/bson"
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

func (r *MongoRepository) GetScreeningCollection() *mongo.Collection {
	return r.client.Database(r.dbName).Collection("slr_screening")
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
