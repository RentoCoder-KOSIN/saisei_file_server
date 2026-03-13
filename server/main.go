package main

import (
    "archive/zip"
    "encoding/json"
    "fmt"
    "io"
    "time"
    "net/http"
    "os"
    "path/filepath"
    "strings"
    "database/sql"
    _ "github.com/mattn/go-sqlite3"
    "golang.org/x/crypto/bcrypt"
)

var db *sql.DB

// アップロードバッファ
var uploadBuffer = map[string][]struct {
    Path string
    Data []byte
}{}

func initDB() {
    var err error
    db, err = sql.Open("sqlite3", "./users.db")
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
    db.Exec(`insert or ignore into users (username, password, role) values (?, ?, ?)`, username, string(hash), role)
}

func addUserHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "POSTのみ対応しています", http.StatusMethodNotAllowed)
        return
    }

    var req struct {
        Username string `json:"username"`
        Password string `json:"password"`
        Role     string `json:"role"`
    }
    json.NewDecoder(r.Body).Decode(&req)

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

    // 現在のパスワードを確認
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

    // 新しいパスワードに更新
    newHash, _ := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
    db.Exec(`UPDATE users SET password = ? WHERE username = ?`, string(newHash), req.Username)

    fmt.Fprintf(w, "✅ パスワードを変更しました")
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "Only POST", http.StatusMethodNotAllowed)
        return
    }
    
    var req struct {
        Username    string  `json:"username"`
        Password    string  `json:"password"`
    }
    json.NewDecoder(r.Body).Decode(&req)
    
    var hash, role string
    err := db.QueryRow(`select password, role from users where username = ?`, req.Username).Scan(&hash, &role)
    if err != nil {
        http.Error(w, "Not found user", http.StatusUnauthorized)
        return
    }

    if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)); err != nil {
        http.Error(w, "not password", http.StatusUnauthorized)
        return
    }
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{
        "username": req.Username,
        "role":     role,
    })
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

    // フォルダ名を取得（例: myproject/main.py → myproject）
    folderName := strings.Split(filePath, "/")[0]

    // ファイルの中身を読み込む
    data, err := io.ReadAll(file)
    if err != nil {
        http.Error(w, "ファイルの読み込みに失敗しました", http.StatusInternalServerError)
        return
    }

    // バッファに追加
    uploadBuffer[folderName] = append(uploadBuffer[folderName], struct {
        Path string
        Data []byte
    }{filePath, data})

    fmt.Fprintf(w, "ok")
}

func finalizeHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "POSTのみ対応しています", http.StatusMethodNotAllowed)
        return
    }

    folderName := r.URL.Query().Get("folder")
    if folderName == "" {
        http.Error(w, "フォルダ名が指定されていません", http.StatusBadRequest)
        return
    }

    files, ok := uploadBuffer[folderName]
    if !ok || len(files) == 0 {
        http.Error(w, "アップロードされたファイルがありません", http.StatusBadRequest)
        return
    }

    // zipファイルを作成
    zipPath := filepath.Join("uploads", folderName+".zip")
    zipFile, err := os.Create(zipPath)
    if err != nil {
        http.Error(w, "zipの作成に失敗しました", http.StatusInternalServerError)
        return
    }
    defer zipFile.Close()

    zw := zip.NewWriter(zipFile)
    defer zw.Close()

    for _, f := range files {
        fw, err := zw.Create(f.Path)
        if err != nil {
            continue
        }
        fw.Write(f.Data)
    }

    // バッファをクリア
    delete(uploadBuffer, folderName)

    fmt.Fprintf(w, "✅ %s.zip を保存しました", folderName)
    
    // DBに記録
    now := time.Now().Format("2006-01-02 15:04:05")
    username := r.URL.Query().Get("username")
    db.Exec(`INSERT INTO uploads (filename, username, uploaded_at) VALUES (?, ?, ?)`,
        folderName+".zip", username, now)
}

type FileInfo struct {
    Filename   string `json:"filename"`
    Username   string `json:"username"`
    UploadedAt string `json:"uploaded_at"`
}

func filesHandler(w http.ResponseWriter, r *http.Request) {
    rows, err := db.Query(`SELECT filename, username, uploaded_at FROM uploads ORDER BY id DESC`)
    if err != nil {
        http.Error(w, "読み込みエラー", http.StatusInternalServerError)
        return
    }
    defer rows.Close()

    var files []FileInfo
    for rows.Next() {
        var f FileInfo
        rows.Scan(&f.Filename, &f.Username, &f.UploadedAt)
        files = append(files, f)
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(files)
}

func deleteHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodDelete {
        http.Error(w, "DELETEのみ対応しています", http.StatusMethodNotAllowed)
        return
    }

    name := r.URL.Query().Get("name")
    if name == "" {
        http.Error(w, "ファイル名が指定されていません", http.StatusBadRequest)
        return
    }

    fullPath := filepath.Join("uploads", name)
    if err := os.Remove(fullPath); err != nil {
        http.Error(w, "削除に失敗しました", http.StatusInternalServerError)
        return
    }

    fmt.Fprintf(w, "✅ %s を削除しました", name)
}

func downloadHandler(w http.ResponseWriter, r *http.Request) {
    name := r.URL.Query().Get("name")
    if name == "" {
        http.Error(w, "ファイル名が指定されていません", http.StatusBadRequest)
        return
    }

    fullPath := filepath.Join("uploads", name)
    
    w.Header().Set("Content-Disposition", "attachment; filename="+name)
    w.Header().Set("Content-Type", "application/zip")
    http.ServeFile(w, r, fullPath)
}

func main() {
    initDB()
    os.MkdirAll("uploads", 0755)

    http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

    http.HandleFunc("/", indexHandler)
    http.HandleFunc("/login", loginHandler)
    http.HandleFunc("/users", addUserHandler)
    http.HandleFunc("/users/passwd", changePasswordHandler)
    http.HandleFunc("/upload", uploadHandler)
    http.HandleFunc("/finalize", finalizeHandler)
    http.HandleFunc("/files", filesHandler)
    http.HandleFunc("/download", downloadHandler)
    http.HandleFunc("/delete", deleteHandler)

    fmt.Println("サーバー起動中... http://localhost:4450")
    http.ListenAndServe(":4450", nil)
}