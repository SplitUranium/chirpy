package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/splituranium/chirpy/internal/auth"
	"github.com/splituranium/chirpy/internal/database"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	queries        *database.Queries
	platform       string
	secret         string
}

type validResponse struct {
	Body string `json:"cleaned_body"`
}

type errorResponse struct {
	Error string `json:"error"`
}

type User struct {
	ID          uuid.UUID `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Email       string    `json:"email"`
	Token       string    `json:"token"`
	IsChirpyRed bool      `json:"is_chirpy_red"`
}

type chirpConv struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
}

var BADWORDS = []string{"kerfuffle", "sharbert", "fornax"}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})

}

func (cfg *apiConfig) handlerWriteRequests(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(fmt.Sprintf(`<html>
  <body>
    <h1>Welcome, Chirpy Admin</h1>
    <p>Chirpy has been visited %d times!</p>
  </body>
</html>`, cfg.fileserverHits.Load())))
}

func (cfg *apiConfig) handlerResetUsers(w http.ResponseWriter, r *http.Request) {
	if cfg.platform != "dev" {
		respondWithError(w, 403, "Forbidden")
	}
	cfg.queries.ResetUsers(r.Context())
}

func respondWithError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	resp := errorResponse{
		Error: msg,
	}
	dat, err := json.Marshal(resp)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		return
	}
	w.Write(dat)
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	dat, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
	}
	w.Write(dat)
}

func replaceBadWords(s string) string {
	sSplit := strings.Split(s, " ")
	for i, word := range sSplit {
		if slices.Contains(BADWORDS, strings.ToLower(word)) {
			sSplit[i] = "****"
		}
	}
	return strings.Join(sSplit, " ")
}

func (cfg *apiConfig) handlerCreateUser(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	params := parameters{}
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Error reading input: %v", err))
		return
	}
	hashedPW, err := auth.HashPassword(params.Password)
	if err != nil {
		respondWithError(w, 500, "Error hashing password")
		return
	}
	createUserParams := database.CreateUserParams{
		Email:          params.Email,
		HashedPassword: hashedPW,
	}
	newUser, err := cfg.queries.CreateUser(r.Context(), createUserParams)
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Error getting email: %v", err))
		return
	}
	respondWithJSON(w, 201, User{
		ID:          newUser.ID,
		CreatedAt:   newUser.CreatedAt,
		UpdatedAt:   newUser.UpdatedAt,
		Email:       newUser.Email,
		IsChirpyRed: newUser.IsChirpyRed,
	})
}

func (cfg *apiConfig) handlerChirp(w http.ResponseWriter, r *http.Request) {

	type parameters struct {
		Body   string    `json:"body"`
		UserID uuid.UUID `json:"user_id"`
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 401, "Error retrieving token")
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.secret)
	if err != nil {
		respondWithError(w, 401, "Validation error")
		return
	}

	params := parameters{}
	decoder := json.NewDecoder(r.Body)
	err = decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Error reading input: %v", err))
		return
	}
	if len(params.Body) > 140 {
		respondWithError(w, 400, "Chirp is too long")
		return
	}

	params.Body = replaceBadWords(params.Body)

	chirpParams := database.CreateChirpParams{
		Body:   params.Body,
		UserID: userID,
	}

	newChirp, err := cfg.queries.CreateChirp(r.Context(), chirpParams)
	if err != nil {
		respondWithError(w, 500, err.Error())
		return
	}

	madeChirp := chirpConv{
		ID:        newChirp.ID,
		CreatedAt: newChirp.CreatedAt,
		UpdatedAt: newChirp.UpdatedAt,
		Body:      newChirp.Body,
		UserID:    newChirp.UserID,
	}

	respondWithJSON(w, 201, madeChirp)
}

func (cfg *apiConfig) handlerGetAllChirps(w http.ResponseWriter, r *http.Request) {
	convChirpList := []chirpConv{}
	chirpList, err := cfg.queries.GetAllChirps(r.Context())
	if err != nil {
		respondWithError(w, 500, "Error getting chirps")
		return
	}
	for _, chirp := range chirpList {
		tempChirp := chirpConv{
			ID:        chirp.ID,
			CreatedAt: chirp.CreatedAt,
			UpdatedAt: chirp.UpdatedAt,
			Body:      chirp.Body,
			UserID:    chirp.UserID,
		}
		convChirpList = append(convChirpList, tempChirp)
	}
	respondWithJSON(w, 200, convChirpList)

}

func (cfg *apiConfig) handlerGetOneChirp(w http.ResponseWriter, r *http.Request) {
	chirpID, err := uuid.Parse(r.PathValue("chirpID"))
	if err != nil {
		respondWithError(w, 404, "Invalid chirp ID")
		return
	}
	chirp, err := cfg.queries.GetOneChirp(r.Context(), chirpID)
	if err != nil {
		respondWithError(w, 404, "Error getting chirp")
		return
	}
	respondWithJSON(w, 200, chirpConv{
		ID:        chirp.ID,
		CreatedAt: chirp.CreatedAt,
		UpdatedAt: chirp.UpdatedAt,
		Body:      chirp.Body,
		UserID:    chirp.UserID,
	})
}

func (cfg *apiConfig) handlerLogin(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	type user struct {
		ID           uuid.UUID `json:"id"`
		CreatedAt    time.Time `json:"created_at"`
		UpdatedAt    time.Time `json:"updated_at"`
		Email        string    `json:"email"`
		Token        string    `json:"token"`
		RefreshToken string    `json:"refresh_token"`
		IsChirpyRed  bool      `json:"is_chirpy_red"`
	}

	loginInfo := parameters{}
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&loginInfo)
	if err != nil {
		respondWithError(w, 401, "Incorrect email or password")
		return
	}

	expiration := time.Hour

	userID, err := cfg.queries.UserIDFromEmail(r.Context(), loginInfo.Email)
	if err != nil {
		respondWithError(w, 401, "Incorrect email or password")
		return
	}

	passCheck, err := auth.CheckPasswordHash(loginInfo.Password, userID.HashedPassword)
	if err != nil || !passCheck {
		respondWithError(w, 401, "Incorrect email or password")
		return
	}

	tokenToUse, err := auth.MakeJWT(userID.ID, cfg.secret, expiration)
	if err != nil {
		respondWithError(w, 500, "Token creation error")
		return
	}

	refreshToken := auth.MakeRefreshToken()
	refreshExpiry := time.Now().Add(time.Hour * 24 * 60)
	refreshParams := database.CreateRefreshTokenParams{
		Token:     refreshToken,
		UserID:    userID.ID,
		ExpiresAt: refreshExpiry,
	}
	err = cfg.queries.CreateRefreshToken(r.Context(), refreshParams)
	if err != nil {
		respondWithError(w, 500, "Refresh token creation error")
		return
	}

	userToReturn := user{
		ID:           userID.ID,
		CreatedAt:    userID.CreatedAt,
		UpdatedAt:    userID.UpdatedAt,
		Email:        userID.Email,
		Token:        tokenToUse,
		RefreshToken: refreshToken,
		IsChirpyRed:  userID.IsChirpyRed,
	}

	respondWithJSON(w, 200, userToReturn)
}

func (cfg *apiConfig) handlerRefresh(w http.ResponseWriter, r *http.Request) {
	type tokenJSON struct {
		Token string `json:"token"`
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 401, "Error retrieving token")
		return
	}
	refreshToken, err := cfg.queries.GetUserFromRefreshToken(r.Context(), token)
	if err != nil || refreshToken.ExpiresAt.Before(time.Now()) || refreshToken.RevokedAt.Valid {
		respondWithError(w, 401, "Error retreiving token")
		return
	}

	expiration := time.Hour
	tokenToUse, err := auth.MakeJWT(refreshToken.UserID, cfg.secret, expiration)
	if err != nil {
		respondWithError(w, 500, "Token creation error")
		return
	}
	returnToken := tokenJSON{
		Token: tokenToUse,
	}
	respondWithJSON(w, 200, returnToken)
}

func (cfg *apiConfig) handlerRevoke(w http.ResponseWriter, r *http.Request) {
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 500, "Token revocation error")
		return
	}
	err = cfg.queries.RevokeToken(r.Context(), token)
	if err != nil {
		respondWithError(w, 500, "Token revocation error")
		return
	}
	respondWithJSON(w, 204, nil)
}

func (cfg *apiConfig) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	type user struct {
		ID           uuid.UUID `json:"id"`
		CreatedAt    time.Time `json:"created_at"`
		UpdatedAt    time.Time `json:"updated_at"`
		Email        string    `json:"email"`
		Token        string    `json:"token"`
		RefreshToken string    `json:"refresh_token"`
		IsChirpyRed  bool      `json:"is_chirpy_red"`
	}
	type updateUserParams struct {
		ID             uuid.UUID `json:"id"`
		Email          string    `json:"email"`
		HashedPassword string    `json:"hashed_password"`
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 401, "Unauthorized")
	}

	userID, err := auth.ValidateJWT(token, cfg.secret)
	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}

	loginInfo := parameters{}
	decoder := json.NewDecoder(r.Body)
	err = decoder.Decode(&loginInfo)
	if err != nil {
		respondWithError(w, 401, "Incorrect email or password")
		return
	}

	hashedNewPassword, err := auth.HashPassword(loginInfo.Password)
	if err != nil {
		respondWithError(w, 500, "Error hashing password")
		return
	}

	updatedUser := updateUserParams{
		ID:             userID,
		Email:          loginInfo.Email,
		HashedPassword: hashedNewPassword,
	}

	newUserInfo, err := cfg.queries.UpdateUser(r.Context(), database.UpdateUserParams(updatedUser))
	if err != nil {
		respondWithError(w, 500, "Error updating information")
		return
	}

	respondWithJSON(w, 200, User{
		ID:          newUserInfo.ID,
		CreatedAt:   newUserInfo.CreatedAt,
		UpdatedAt:   newUserInfo.UpdatedAt,
		Email:       newUserInfo.Email,
		IsChirpyRed: newUserInfo.IsChirpyRed,
	})
}

func (cfg *apiConfig) handleDeleteChirps(w http.ResponseWriter, r *http.Request) {
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 401, "Unauthorized")
	}

	userID, err := auth.ValidateJWT(token, cfg.secret)
	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}

	chirpID, err := uuid.Parse(r.PathValue("chirpID"))
	if err != nil {
		respondWithError(w, 404, "Invalid chirp ID")
		return
	}

	chirp, err := cfg.queries.GetOneChirp(r.Context(), chirpID)
	if err != nil {
		respondWithError(w, 404, "Error getting chirp")
		return
	}

	if chirp.UserID != userID {
		respondWithError(w, 403, "You are not the owner of this chirp")
		return
	}

	err = cfg.queries.DeleteChirp(r.Context(), chirpID)
	if err != nil {
		respondWithError(w, 401, "Error deleting chirp")
		return
	}

	respondWithJSON(w, 204, "Success")
}

func (cfg *apiConfig) handlePolkaWebhooks(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Event string `json:"event"`
		Data  struct {
			UserID uuid.UUID `json:"user_id"`
		} `json:"data"`
	}

	webhookData := parameters{}
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&webhookData)
	if err != nil {
		respondWithError(w, 401, "Incorrect webhook")
		return
	}

	if webhookData.Event != "user.upgraded" {
		respondWithError(w, 204, "User not upgraded")
		return
	}

	err = cfg.queries.UpgradeChirpyRed(r.Context(), webhookData.Data.UserID)
	if err != nil {
		respondWithError(w, 404, "User not found")
		return
	}

	respondWithJSON(w, 204, nil)
}

func main() {
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	platform := os.Getenv("PLATFORM")
	secret := os.Getenv("SECRET")
	db, err := sql.Open("postgres", dbURL)
	dbQueries := database.New(db)

	var apiCfg = apiConfig{
		fileserverHits: atomic.Int32{},
		queries:        dbQueries,
		platform:       platform,
		secret:         secret,
	}

	apiCfg.fileserverHits.Store(0)
	mux := http.NewServeMux()
	server := http.Server{
		Handler: mux,
		Addr:    ":8080",
	}

	mux.Handle("/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir(".")))))

	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(200)
		body := []byte("OK")
		w.Write(body)
	})

	mux.HandleFunc("GET /admin/metrics", apiCfg.handlerWriteRequests)
	mux.HandleFunc("POST /admin/reset", apiCfg.handlerResetUsers)
	mux.HandleFunc("POST /api/users", apiCfg.handlerCreateUser)
	mux.HandleFunc("POST /api/chirps", apiCfg.handlerChirp)
	mux.HandleFunc("GET /api/chirps", apiCfg.handlerGetAllChirps)
	mux.HandleFunc("GET /api/chirps/{chirpID}", apiCfg.handlerGetOneChirp)
	mux.HandleFunc("POST /api/login", apiCfg.handlerLogin)
	mux.HandleFunc("POST /api/refresh", apiCfg.handlerRefresh)
	mux.HandleFunc("POST /api/revoke", apiCfg.handlerRevoke)
	mux.HandleFunc("PUT /api/users", apiCfg.handleUpdateUser)
	mux.HandleFunc("DELETE /api/chirps/{chirpID}", apiCfg.handleDeleteChirps)
	mux.HandleFunc("POST /api/polka/webhooks", apiCfg.handlePolkaWebhooks)

	err = server.ListenAndServe()
	if err != nil {
		log.Printf("Error starting server: %v\n", err)
	}
}
