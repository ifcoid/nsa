package http

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"nsa/internal/model"
	"nsa/internal/repository"

	"aidanwoods.dev/go-paseto"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	mongoRepo *repository.MongoRepository
}

func NewAuthHandler(mongoRepo *repository.MongoRepository) *AuthHandler {
	return &AuthHandler{
		mongoRepo: mongoRepo,
	}
}

// Register creates a new user
func (h *AuthHandler) Register(w http.ResponseWriter, req *http.Request) {
	var payload struct {
		Username   string `json:"username"`
		Password   string `json:"password"`
		InviteCode string `json:"invite_code,omitempty"`
	}

	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		sendJSONError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if payload.Username == "" || payload.Password == "" {
		sendJSONError(w, http.StatusBadRequest, "Username and password are required")
		return
	}

	// Verify invite code if env var is set
	expectedInviteCode := os.Getenv("REGISTER_INVITE_CODE")
	if expectedInviteCode != "" && payload.InviteCode != expectedInviteCode {
		sendJSONError(w, http.StatusForbidden, "Invalid invite code")
		return
	}

	ctx := context.Background()

	// Cek apakah username sudah ada
	existingUser, _ := h.mongoRepo.GetUserByUsername(ctx, payload.Username)
	if existingUser != nil {
		sendJSONError(w, http.StatusConflict, "Username already exists")
		return
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(payload.Password), bcrypt.DefaultCost)
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Failed to hash password")
		return
	}

	user := &model.User{
		ID:       primitive.NewObjectID(),
		Username: payload.Username,
		Password: string(hashedPassword),
		Role:     "admin", // default role
	}

	if err := h.mongoRepo.CreateUser(ctx, user); err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Failed to create user")
		return
	}

	sendJSONResponse(w, http.StatusCreated, map[string]string{
		"message": "User registered successfully",
	})
}

// Login verifies credentials and returns a PASETO v4 token
func (h *AuthHandler) Login(w http.ResponseWriter, req *http.Request) {
	var payload struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		sendJSONError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	ctx := context.Background()

	// Cek user
	user, err := h.mongoRepo.GetUserByUsername(ctx, payload.Username)
	if err != nil || user == nil {
		sendJSONError(w, http.StatusUnauthorized, "Invalid username or password")
		return
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(payload.Password)); err != nil {
		sendJSONError(w, http.StatusUnauthorized, "Invalid username or password")
		return
	}

	// Get PASETO Private Key dari env
	privateKeyHex := os.Getenv("PASETO_PRIVATE_KEY")
	if privateKeyHex == "" {
		sendJSONError(w, http.StatusInternalServerError, "Server configuration error: PASETO_PRIVATE_KEY not set")
		return
	}

	secretKey, err := paseto.NewV4AsymmetricSecretKeyFromHex(privateKeyHex)
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, "Server configuration error: Invalid PASETO_PRIVATE_KEY")
		return
	}

	// Buat Token PASETO v4
	token := paseto.NewToken()
	token.SetIssuer("agentic-slr")
	token.SetSubject(user.ID.Hex())
	token.SetString("username", user.Username)
	token.SetString("role", user.Role)
	token.SetIssuedAt(time.Now())
	token.SetExpiration(time.Now().Add(24 * time.Hour)) // 24 hours expiry

	// Sign Token
	signed := token.V4Sign(secretKey, nil)

	// Jangan kirim password kembali!
	userData := map[string]string{
		"id":       user.ID.Hex(),
		"username": user.Username,
		"role":     user.Role,
	}

	sendJSONResponse(w, http.StatusOK, map[string]interface{}{
		"message":    "Login successful",
		"auth_token": signed,
		"user_data":  userData,
	})
}
