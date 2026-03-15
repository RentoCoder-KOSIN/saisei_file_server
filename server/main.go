package main

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

var db *sql.DB

var taskServerURL = getEnv("TASK_SERVER_URL", "http://task-server:8000")

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

var (
	uploadBuffer = map[string][]struct {
		Path string
		Data []byte
	}{}
	uploadMu sync.Mutex
)

type FileInfo struct {
	Filename   string `json:"filename"`
	Username   string `json:"username"`
	UploadedAt string `json:"uploaded_at"`
}

// ══════════════════════════════════════
//  ロール認可ヘルパー
// ══════════════════════════════════════

func getRole(username string) (string, error) {
	var role string
	err := db.QueryRow(`SELECT role FROM users WHERE username = ?`, username).Scan(&role)
	return role, err
}

// teacherでなければ403を返してfalseを返す
func requireTeacher(w http.ResponseWriter, username string) bool {
	if username == "" {
		http.Error(w, "操作者のusernameが必要です", http.StatusBadRequest)
		return false
	}
	role, err := getRole(username)
	if err != nil || role != "teacher" {
		http.Error(w, "先生のみ操作できます", http.StatusForbidden)
		return false
	}
	return true
}

// ══════════════════════════════════════
//  DB初期化
// ══════════════════════════════════════

func initDB() {
	var err error
	db, err = sql.Open("sqlite3", getEnv("DB_PATH", "./users.db"))
	if err != nil {
		panic(err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id       INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT UNIQUE NOT NULL,
			password TEXT NOT NULL,
			role     TEXT NOT NULL DEFAULT 'student'
		)
	`)
	if err != nil {
		panic(err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS uploads (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			filename    TEXT NOT NULL,
			username    TEXT NOT NULL,
			uploaded_at TEXT NOT NULL
		)
	`)
	if err != nil {
		panic(err)
	}

	createUserIfNotExists("admin", "admin123", "teacher")
}

func createUserIfNotExists(username, password, role string) {
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	db.Exec(`INSERT OR IGNORE INTO users (username, password, role) VALUES (?, ?, ?)`,
		username, string(hash), role)
}

// ══════════════════════════════════════
//  ハンドラ
// ══════════════════════════════════════

func loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POSTのみ対応しています", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	var hash, role string
	err := db.QueryRow(`SELECT password, role FROM users WHERE username = ?`, req.Username).Scan(&hash, &role)
	if err != nil {
		http.Error(w, "ユーザーが見つかりません", http.StatusUnauthorized)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)); err != nil {
		http.Error(w, "パスワードが違います", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"username": req.Username, "role": role})
}

func usersHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		listUsersHandler(w, r)
	case http.MethodPost:
		addUserHandler(w, r)
	case http.MethodDelete:
		deleteUserHandler(w, r)
	default:
		http.Error(w, "未対応のメソッドです", http.StatusMethodNotAllowed)
	}
}

func listUsersHandler(w http.ResponseWriter, r *http.Request) {
	// 認可チェック: ?operator=<currentUser>
	if !requireTeacher(w, r.URL.Query().Get("operator")) {
		return
	}

	rows, err := db.Query(`SELECT username, role FROM users ORDER BY role, username`)
	if err != nil {
		http.Error(w, "読み込みエラー", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type UserInfo struct {
		Username string `json:"username"`
		Role     string `json:"role"`
	}
	var users []UserInfo
	for rows.Next() {
		var u UserInfo
		rows.Scan(&u.Username, &u.Role)
		users = append(users, u)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}

func addUserHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Operator string `json:"operator"` // 操作者（先生）のusername
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	// 認可チェック
	if !requireTeacher(w, req.Operator) {
		return
	}

	if req.Username == "" || req.Password == "" {
		http.Error(w, "username・passwordは必須です", http.StatusBadRequest)
		return
	}
	if req.Role == "" {
		req.Role = "student"
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	_, err := db.Exec(`INSERT INTO users (username, password, role) VALUES (?, ?, ?)`,
		req.Username, string(hash), req.Role)
	if err != nil {
		http.Error(w, "ユーザーがすでに存在します", http.StatusConflict)
		return
	}
	fmt.Fprintf(w, "✅ %s を追加しました", req.Username)
}

func deleteUserHandler(w http.ResponseWriter, r *http.Request) {
	// 認可チェック: ?operator=<currentUser>&username=<target>
	if !requireTeacher(w, r.URL.Query().Get("operator")) {
		return
	}

	name := r.URL.Query().Get("username")
	if name == "" {
		http.Error(w, "ユーザー名が指定されていません", http.StatusBadRequest)
		return
	}
	if name == "admin" {
		http.Error(w, "adminは削除できません", http.StatusForbidden)
		return
	}
	res, err := db.Exec(`DELETE FROM users WHERE username = ?`, name)
	if err != nil {
		http.Error(w, "削除に失敗しました", http.StatusInternalServerError)
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		http.Error(w, "ユーザーが見つかりません", http.StatusNotFound)
		return
	}
	fmt.Fprintf(w, "✅ %s を削除しました", name)
}

func changePasswordHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "PUTのみ対応しています", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Username    string `json:"username"`
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	var hash string
	err := db.QueryRow(`SELECT password FROM users WHERE username = ?`, req.Username).Scan(&hash)
	if err != nil {
		http.Error(w, "ユーザーが見つかりません", http.StatusUnauthorized)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.OldPassword)); err != nil {
		http.Error(w, "現在のパスワードが違います", http.StatusUnauthorized)
		return
	}
	newHash, _ := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	db.Exec(`UPDATE users SET password = ? WHERE username = ?`, string(newHash), req.Username)
	fmt.Fprintf(w, "✅ パスワードを変更しました")
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "static/index.html")
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "ファイルが取得できません", http.StatusBadRequest)
		return
	}
	defer file.Close()

	filePath := r.FormValue("path")
	if filePath == "" {
		http.Error(w, "パスが指定されていません", http.StatusBadRequest)
		return
	}

	folderName := strings.Split(filePath, "/")[0]
	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "ファイルの読み込みに失敗しました", http.StatusInternalServerError)
		return
	}
	uploadMu.Lock()
	uploadBuffer[folderName] = append(uploadBuffer[folderName], struct {
		Path string
		Data []byte
	}{filePath, data})
	uploadMu.Unlock()
	fmt.Fprintf(w, "ok")
}

func finalizeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POSTのみ対応しています", http.StatusMethodNotAllowed)
		return
	}

	folderName := r.URL.Query().Get("folder")
	username   := r.URL.Query().Get("username")
	taskID     := r.URL.Query().Get("task_id")

	if folderName == "" {
		http.Error(w, "フォルダ名が指定されていません", http.StatusBadRequest)
		return
	}

	uploadMu.Lock()
	files, ok := uploadBuffer[folderName]
	delete(uploadBuffer, folderName)
	uploadMu.Unlock()

	if !ok || len(files) == 0 {
		http.Error(w, "アップロードされたファイルがありません", http.StatusBadRequest)
		return
	}

	if username == "" {
		http.Error(w, "usernameが指定されていません", http.StatusBadRequest)
		return
	}

	// サブディレクトリ: task_id が未指定なら "free"
	subDir := taskID
	if subDir == "" {
		subDir = "free"
	}

	// uploads/{username}/{task_id or free}/ ディレクトリを作成
	userDir := filepath.Join("uploads", username, subDir)
	if err := os.MkdirAll(userDir, 0755); err != nil {
		http.Error(w, "ディレクトリの作成に失敗しました", http.StatusInternalServerError)
		return
	}

	zipPath := filepath.Join(userDir, folderName+".zip")
	zipFile, err := os.Create(zipPath)
	if err != nil {
		http.Error(w, "zipの作成に失敗しました", http.StatusInternalServerError)
		return
	}

	// defer の逆順実行バグ回避: zw → zipFile の順で明示的にクローズ
	zw := zip.NewWriter(zipFile)
	for _, f := range files {
		fw, err := zw.Create(f.Path)
		if err != nil {
			continue
		}
		fw.Write(f.Data)
	}
	zw.Close()
	zipFile.Close()

	// DBには {username}/{task_id or free}/{folder}.zip 形式で保存
	storedName := username + "/" + subDir + "/" + folderName + ".zip"
	now := time.Now().Format("2006-01-02 15:04:05")
	db.Exec(`INSERT INTO uploads (filename, username, uploaded_at) VALUES (?, ?, ?)`,
		storedName, username, now)

	if taskID != "" {
		go notifySubmission(taskID, username, storedName)
	}

	fmt.Fprintf(w, "✅ %s.zip を保存しました", folderName)
}

func notifySubmission(taskID, username, filename string) {
	type submissionPayload struct {
		TaskID   string `json:"task_id"`
		Username string `json:"username"`
		Filename string `json:"filename"`
	}
	payload, _ := json.Marshal(submissionPayload{TaskID: taskID, Username: username, Filename: filename})
	resp, err := http.Post(taskServerURL+"/submissions", "application/json", bytes.NewReader(payload))
	if err != nil {
		fmt.Printf("⚠️ タスクサーバー通知失敗: %v\n", err)
		return
	}
	defer resp.Body.Close()
	fmt.Printf("✅ 提出通知完了: task=%s user=%s\n", taskID, username)
}

func filesHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`SELECT filename, username, uploaded_at FROM uploads ORDER BY id DESC`)
	if err != nil {
		http.Error(w, "読み込みエラー", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var fileList []FileInfo
	for rows.Next() {
		var f FileInfo
		rows.Scan(&f.Filename, &f.Username, &f.UploadedAt)
		fileList = append(fileList, f)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(fileList)
}

func deleteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "DELETEのみ対応しています", http.StatusMethodNotAllowed)
		return
	}
	if !requireTeacher(w, r.URL.Query().Get("operator")) {
		return
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		http.Error(w, "ファイル名が指定されていません", http.StatusBadRequest)
		return
	}
	// パストラバーサル対策
	fullPath := filepath.Join("uploads", filepath.Clean("/"+name))
	if !strings.HasPrefix(fullPath, filepath.Clean("uploads")+string(os.PathSeparator)) {
		http.Error(w, "不正なパスです", http.StatusBadRequest)
		return
	}
	if err := os.Remove(fullPath); err != nil {
		http.Error(w, "削除に失敗しました", http.StatusInternalServerError)
		return
	}
	db.Exec(`DELETE FROM uploads WHERE filename = ?`, name)
	fmt.Fprintf(w, "✅ %s を削除しました", name)
}

func downloadHandler(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		http.Error(w, "ファイル名が指定されていません", http.StatusBadRequest)
		return
	}
	// パストラバーサル対策
	fullPath := filepath.Join("uploads", filepath.Clean("/"+name))
	if !strings.HasPrefix(fullPath, filepath.Clean("uploads")+string(os.PathSeparator)) {
		http.Error(w, "不正なパスです", http.StatusBadRequest)
		return
	}
	// Content-Disposition はファイル名部分のみ（パスを含まない）
	w.Header().Set("Content-Disposition", "attachment; filename="+filepath.Base(name))
	w.Header().Set("Content-Type", "application/zip")
	http.ServeFile(w, r, fullPath)
}

func unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)
		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, 0755)
			continue
		}
		os.MkdirAll(filepath.Dir(fpath), 0755)
		outFile, err := os.Create(fpath)
		if err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
	}
	return nil
}

func runHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POSTのみ対応しています", http.StatusMethodNotAllowed)
		return
	}
	// 認可チェック: ?operator=<currentUser>&name=<filename>
	if !requireTeacher(w, r.URL.Query().Get("operator")) {
		return
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		http.Error(w, "ファイル名が指定されていません", http.StatusBadRequest)
		return
	}
	// パストラバーサル対策
	zipPath := filepath.Join("uploads", filepath.Clean("/"+name))
	if !strings.HasPrefix(zipPath, filepath.Clean("uploads")+string(os.PathSeparator)) {
		http.Error(w, "不正なパスです", http.StatusBadRequest)
		return
	}
	tmpDir, err := os.MkdirTemp("", "pylab-*")
	if err != nil {
		http.Error(w, "一時フォルダの作成に失敗しました", http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(tmpDir)

	if err := unzip(zipPath, tmpDir); err != nil {
		http.Error(w, "zip解凍に失敗しました: "+err.Error(), http.StatusInternalServerError)
		return
	}

	mainPy := ""
	filepath.Walk(tmpDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.Name() == "main.py" {
			mainPy = path
		}
		return nil
	})
	if mainPy == "" {
		http.Error(w, "main.pyが見つかりません", http.StatusBadRequest)
		return
	}

	codeDir := filepath.Dir(mainPy)

	// タイムアウト設定（環境変数 RUN_TIMEOUT_SEC、デフォルト30秒）
	timeoutSec := 30
	if v := os.Getenv("RUN_TIMEOUT_SEC"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			timeoutSec = n
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "run", "--rm",
		"--network", "none",
		"--memory", "128m",
		"-v", codeDir+":/code",
		"pylab-python",
		"python3", "/code/main.py",
	)
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	cmd.Run()

	// タイムアウト時はstderrにメッセージを追加
	stderr := errOut.String()
	if ctx.Err() == context.DeadlineExceeded {
		stderr += fmt.Sprintf("\n⏱ 実行がタイムアウトしました（%d秒）", timeoutSec)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"stdout": out.String(),
		"stderr": stderr,
	})
}

// ══════════════════════════════════════
//  /api/* → task-server リバースプロキシ
// ══════════════════════════════════════

func apiProxyHandler(w http.ResponseWriter, r *http.Request) {
	target, err := url.Parse(taskServerURL)
	if err != nil {
		http.Error(w, "プロキシ設定エラー", http.StatusInternalServerError)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	// /api/tasks → /tasks のようにプレフィックスを除去して転送
	r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
	if r.URL.Path == "" {
		r.URL.Path = "/"
	}

	// エラー時のハンドリング
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, "タスクサーバーに接続できません: "+err.Error(), http.StatusBadGateway)
	}

	proxy.ServeHTTP(w, r)
}

func main() {
	initDB()
	os.MkdirAll("uploads", 0755)

	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	http.Handle("/api/", http.HandlerFunc(apiProxyHandler)) // ② タスクサーバーへのプロキシ
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/users", usersHandler)
	http.HandleFunc("/users/passwd", changePasswordHandler)
	http.HandleFunc("/upload", uploadHandler)
	http.HandleFunc("/finalize", finalizeHandler)
	http.HandleFunc("/files", filesHandler)
	http.HandleFunc("/download", downloadHandler)
	http.HandleFunc("/delete", deleteHandler)
	http.HandleFunc("/run", runHandler)

	fmt.Println("サーバー起動中... http://localhost:4450")
	http.ListenAndServe(":4450", nil)
}