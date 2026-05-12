package main

// ══════════════════════════════════════
//  軽量JSONファイルDB（外部依存ゼロ）
//  modernc.org/sqlite の代替
//  users.json / uploads.json で永続化
// ══════════════════════════════════════

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// ── データ構造 ────────────────────────────────────────────────────────

type User struct {
	Username string `json:"username"`
	Salt     string `json:"salt"`
	Hash     string `json:"hash"` // sha256(salt + password) の hex
	Role     string `json:"role"`
}

type Upload struct {
	ID         int    `json:"id"`
	Filename   string `json:"filename"`
	Username   string `json:"username"`
	UploadedAt string `json:"uploaded_at"`
}

// ── インメモリDB ──────────────────────────────────────────────────────

var (
	dbMu         sync.RWMutex
	usersDB      []User
	uploadsDB    []Upload
	nextUploadID = 1

	dbDir = "."
)

func dbPath(name string) string {
	return filepath.Join(dbDir, name)
}

// ── パスワードハッシュ（標準ライブラリのみ）────────────────────────

func hashPassword(salt, password string) string {
	h := sha256.New()
	h.Write([]byte(salt + ":" + password))
	return hex.EncodeToString(h.Sum(nil))
}

func generateSalt() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func checkPassword(u User, password string) bool {
	return u.Hash == hashPassword(u.Salt, password)
}

// ── 初期化 ────────────────────────────────────────────────────────────

func initDB() {
	if p := getEnv("DB_PATH", ""); p != "" {
		dbDir = filepath.Dir(p)
	}
	os.MkdirAll(dbDir, 0755)

	loadUsers()
	loadUploads()

	createUserIfNotExists("admin", "admin123", "teacher")
}

// ── Users ─────────────────────────────────────────────────────────────

func loadUsers() {
	data, err := os.ReadFile(dbPath("users.json"))
	if err != nil {
		return
	}
	json.Unmarshal(data, &usersDB)
}

func saveUsers() {
	data, _ := json.MarshalIndent(usersDB, "", "  ")
	os.WriteFile(dbPath("users.json"), data, 0644)
}

func createUserIfNotExists(username, password, role string) {
	dbMu.Lock()
	defer dbMu.Unlock()
	for _, u := range usersDB {
		if u.Username == username {
			return
		}
	}
	salt := generateSalt()
	usersDB = append(usersDB, User{
		Username: username,
		Salt:     salt,
		Hash:     hashPassword(salt, password),
		Role:     role,
	})
	saveUsers()
}

func getRole(username string) (string, error) {
	dbMu.RLock()
	defer dbMu.RUnlock()
	for _, u := range usersDB {
		if u.Username == username {
			return u.Role, nil
		}
	}
	return "", fmt.Errorf("user not found")
}

func authUser(username, password string) (string, error) {
	dbMu.RLock()
	defer dbMu.RUnlock()
	for _, u := range usersDB {
		if u.Username == username {
			if !checkPassword(u, password) {
				return "", fmt.Errorf("wrong password")
			}
			return u.Role, nil
		}
	}
	return "", fmt.Errorf("user not found")
}

func listUsers() []User {
	dbMu.RLock()
	defer dbMu.RUnlock()
	result := make([]User, len(usersDB))
	copy(result, usersDB)
	sort.Slice(result, func(i, j int) bool {
		if result[i].Role != result[j].Role {
			return result[i].Role < result[j].Role
		}
		return result[i].Username < result[j].Username
	})
	return result
}

func addUser(username, password, role string) error {
	dbMu.Lock()
	defer dbMu.Unlock()
	for _, u := range usersDB {
		if u.Username == username {
			return fmt.Errorf("already exists")
		}
	}
	salt := generateSalt()
	usersDB = append(usersDB, User{
		Username: username,
		Salt:     salt,
		Hash:     hashPassword(salt, password),
		Role:     role,
	})
	saveUsers()
	return nil
}

func deleteUser(username string) (bool, error) {
	dbMu.Lock()
	defer dbMu.Unlock()
	for i, u := range usersDB {
		if u.Username == username {
			usersDB = append(usersDB[:i], usersDB[i+1:]...)
			saveUsers()
			return true, nil
		}
	}
	return false, nil
}

func changePassword(username, oldPassword, newPassword string) error {
	dbMu.Lock()
	defer dbMu.Unlock()
	for i, u := range usersDB {
		if u.Username == username {
			if !checkPassword(u, oldPassword) {
				return fmt.Errorf("wrong password")
			}
			salt := generateSalt()
			usersDB[i].Salt = salt
			usersDB[i].Hash = hashPassword(salt, newPassword)
			saveUsers()
			return nil
		}
	}
	return fmt.Errorf("user not found")
}

// ── Uploads ───────────────────────────────────────────────────────────

func loadUploads() {
	data, err := os.ReadFile(dbPath("uploads.json"))
	if err != nil {
		return
	}
	json.Unmarshal(data, &uploadsDB)
	for _, u := range uploadsDB {
		if u.ID >= nextUploadID {
			nextUploadID = u.ID + 1
		}
	}
}

func saveUploads() {
	data, _ := json.MarshalIndent(uploadsDB, "", "  ")
	os.WriteFile(dbPath("uploads.json"), data, 0644)
}

func insertUpload(filename, username string) {
	dbMu.Lock()
	defer dbMu.Unlock()
	uploadsDB = append(uploadsDB, Upload{
		ID:         nextUploadID,
		Filename:   filename,
		Username:   username,
		UploadedAt: time.Now().In(jst()).Format("2006-01-02 15:04:05"),
	})
	nextUploadID++
	saveUploads()
}

func listUploads() []Upload {
	dbMu.RLock()
	defer dbMu.RUnlock()
	result := make([]Upload, len(uploadsDB))
	copy(result, uploadsDB)
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID > result[j].ID
	})
	return result
}

func deleteUpload(filename string) {
	dbMu.Lock()
	defer dbMu.Unlock()
	for i, u := range uploadsDB {
		if u.Filename == filename {
			uploadsDB = append(uploadsDB[:i], uploadsDB[i+1:]...)
			saveUploads()
			return
		}
	}
}

func jst() *time.Location {
	loc, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		return time.FixedZone("JST", 9*60*60)
	}
	return loc
}
