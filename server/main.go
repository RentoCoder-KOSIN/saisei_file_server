package main

import (
    "archive/zip"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "os"
    "path/filepath"
    "strings"
)

// アップロードバッファ
var uploadBuffer = map[string][]struct {
    Path string
    Data []byte
}{}

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
}

func filesHandler(w http.ResponseWriter, r *http.Request) {
    var names []string

    entries, err := os.ReadDir("uploads")
    if err != nil {
        http.Error(w, "読み込みエラー", http.StatusInternalServerError)
        return
    }

    for _, e := range entries {
        if !e.IsDir() {
            names = append(names, e.Name())
        }
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(names)
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
    os.MkdirAll("uploads", 0755)

    http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

    http.HandleFunc("/", indexHandler)
    http.HandleFunc("/upload", uploadHandler)
    http.HandleFunc("/finalize", finalizeHandler)
    http.HandleFunc("/files", filesHandler)
    http.HandleFunc("/download", downloadHandler)
    http.HandleFunc("/delete", deleteHandler)

    fmt.Println("サーバー起動中... http://localhost:4450")
    http.ListenAndServe(":4450", nil)
}