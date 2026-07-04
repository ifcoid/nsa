package http

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Figur bibliometrik/SLNA (di-generate PEDE, credential-safe) diunggah user via Ruang Ekspor
// lalu disajikan sebagai artefak — menutup rantai dokumentasi (figur + data ter-arsip, bukan
// screenshot manual). Disimpan di koleksi slr_figures (1 dok per file), upsert by (session,name).

func figContentType(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".pdf":
		return "application/pdf"
	case ".csv":
		return "text/csv; charset=utf-8"
	case ".json":
		return "application/json"
	default:
		return "application/octet-stream"
	}
}

// UploadFigures: POST multipart (field "files") → simpan tiap file ke slr_figures.
func (h *SessionHandler) UploadFigures(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID required")
		return
	}
	if err := req.ParseMultipartForm(50 << 20); err != nil {
		sendJSONError(w, http.StatusBadRequest, "Gagal mem-parse form (maks 50MB)")
		return
	}
	files := req.MultipartForm.File["files"]
	if len(files) == 0 {
		sendJSONError(w, http.StatusBadRequest, "Tidak ada file diunggah")
		return
	}
	coll := h.mongoRepo.GetFiguresCollection()
	ctx := context.Background()
	saved := 0
	for _, fh := range files {
		f, err := fh.Open()
		if err != nil {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(f, 20<<20)) // maks 20MB/file
		f.Close()
		if err != nil || len(data) == 0 {
			continue
		}
		name := path.Base(fh.Filename)
		_, err = coll.UpdateOne(ctx,
			bson.M{"session_id": id, "filename": name},
			bson.M{"$set": bson.M{
				"session_id":   id,
				"filename":     name,
				"content_type": figContentType(name),
				"data_b64":     base64.StdEncoding.EncodeToString(data),
				"size":         len(data),
				"uploaded_at":  time.Now().UTC().Format(time.RFC3339),
			}},
			options.Update().SetUpsert(true))
		if err == nil {
			saved++
		}
	}
	sendJSONResponse(w, http.StatusOK, map[string]interface{}{"saved": saved, "total": len(files)})
}

// ListFigures: GET → daftar figur (tanpa data) untuk Ruang Ekspor.
func (h *SessionHandler) ListFigures(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID required")
		return
	}
	coll := h.mongoRepo.GetFiguresCollection()
	cur, err := coll.Find(context.Background(),
		bson.M{"session_id": id},
		options.Find().SetProjection(bson.M{"data_b64": 0}).SetSort(bson.M{"filename": 1}))
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Gagal membaca figur")
		return
	}
	var docs []bson.M
	_ = cur.All(context.Background(), &docs)
	out := make([]map[string]interface{}, 0, len(docs))
	for _, d := range docs {
		out = append(out, map[string]interface{}{
			"filename":     d["filename"],
			"content_type": d["content_type"],
			"size":         d["size"],
			"uploaded_at":  d["uploaded_at"],
		})
	}
	sendJSONResponse(w, http.StatusOK, map[string]interface{}{"figures": out})
}

// GetFigure: GET .../figures/{name} → sajikan satu file figur.
func (h *SessionHandler) GetFigure(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	name := path.Base(req.PathValue("name"))
	if id == "" || name == "" {
		sendJSONError(w, http.StatusBadRequest, "Session ID + nama file required")
		return
	}
	var doc bson.M
	err := h.mongoRepo.GetFiguresCollection().FindOne(context.Background(),
		bson.M{"session_id": id, "filename": name}).Decode(&doc)
	if err != nil {
		sendJSONError(w, http.StatusNotFound, "Figur tak ditemukan")
		return
	}
	b64, _ := doc["data_b64"].(string)
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Data figur rusak")
		return
	}
	ct, _ := doc["content_type"].(string)
	if ct == "" {
		ct = figContentType(name)
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, name))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
