package main

import (
	"archive/zip"
	"bytes"
	"context"
	_ "embed"
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
)

//go:embed pyrunner.py
var pyrunnerPy []byte

var taskServerURL = getEnv("TASK_SERVER_URL", "http://task-server:8000")

// Docker-outside-of-Docker 対策:
// コンテナ内の /app/uploads はホスト側では HOST_UPLOAD_DIR のパスにある。
var hostUploadDir = getEnv("HOST_UPLOAD_DIR", "")

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

	role, err := authUser(req.Username, req.Password)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "ユーザーが見つかりません", http.StatusUnauthorized)
		} else {
			http.Error(w, "パスワードが違います", http.StatusUnauthorized)
		}
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
	if !requireTeacher(w, r.URL.Query().Get("operator")) {
		return
	}

	users := listUsers()
	type UserInfo struct {
		Username string `json:"username"`
		Role     string `json:"role"`
	}
	var result []UserInfo
	for _, u := range users {
		result = append(result, UserInfo{Username: u.Username, Role: u.Role})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func addUserHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Operator string `json:"operator"`
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	json.NewDecoder(r.Body).Decode(&req)

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
	if err := addUser(req.Username, req.Password, req.Role); err != nil {
		http.Error(w, "ユーザーがすでに存在します", http.StatusConflict)
		return
	}
	fmt.Fprintf(w, "✅ %s を追加しました", req.Username)
}

func deleteUserHandler(w http.ResponseWriter, r *http.Request) {
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
	found, err := deleteUser(name)
	if err != nil {
		http.Error(w, "削除に失敗しました", http.StatusInternalServerError)
		return
	}
	if !found {
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

	if err := changePassword(req.Username, req.OldPassword, req.NewPassword); err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "ユーザーが見つかりません", http.StatusUnauthorized)
		} else {
			http.Error(w, "現在のパスワードが違います", http.StatusUnauthorized)
		}
		return
	}
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

	subDir := taskID
	if subDir == "" {
		subDir = "free"
	}

	userDir := filepath.Join("uploads", username, subDir)
	if err := os.MkdirAll(userDir, 0755); err != nil {
		http.Error(w, "ディレクトリの作成に失敗しました", http.StatusInternalServerError)
		return
	}

	// 同じ課題の古い提出zipを削除（ディスク節約）
	if taskID != "" {
		if oldFiles, err := filepath.Glob(filepath.Join(userDir, "*.zip")); err == nil {
			for _, old := range oldFiles {
				oldName := username + "/" + subDir + "/" + filepath.Base(old)
				os.Remove(old)
				deleteUpload(oldName)
			}
		}
	}

	// 同じ課題への複数提出を許可するためタイムスタンプを付与
	timestamp := time.Now().In(func() *time.Location {
		loc, err := time.LoadLocation("Asia/Tokyo")
		if err != nil {
			return time.FixedZone("JST", 9*60*60)
		}
		return loc
	}()).Format("20060102_150405")
	zipFilename := folderName + "_" + timestamp + ".zip"
	zipPath := filepath.Join(userDir, zipFilename)
	zipFile, err := os.Create(zipPath)
	if err != nil {
		http.Error(w, "zipの作成に失敗しました", http.StatusInternalServerError)
		return
	}

	zw := zip.NewWriter(zipFile)
	for _, f := range files {
		fw, err := zw.Create(f.Path)
		if err != nil {
			continue
		}
		fw.Write(f.Data)
	}

	// run.bat と pyrunner.py をプロジェクトフォルダ内に同梱
	if runBat := buildRunBat(); runBat != "" {
		if fw, err := zw.Create(folderName + "/run.bat"); err == nil {
			fw.Write([]byte(runBat))
		}
	}
	if fw, err := zw.Create(folderName + "/pyrunner.py"); err == nil {
		fw.Write(pyrunnerPy)
	}

	zw.Close()
	zipFile.Close()

	storedName := username + "/" + subDir + "/" + zipFilename
	insertUpload(storedName, username)

	if taskID != "" {
		go notifySubmission(taskID, username, storedName)
	}

	fmt.Fprintf(w, "✅ %s を保存しました", zipFilename)
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
	uploads := listUploads()
	var fileList []FileInfo
	for _, u := range uploads {
		fileList = append(fileList, FileInfo{
			Filename:   u.Filename,
			Username:   u.Username,
			UploadedAt: u.UploadedAt,
		})
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
	fullPath := filepath.Join("uploads", filepath.Clean("/"+name))
	if !strings.HasPrefix(fullPath, filepath.Clean("uploads")+string(os.PathSeparator)) {
		http.Error(w, "不正なパスです", http.StatusBadRequest)
		return
	}
	if err := os.Remove(fullPath); err != nil {
		http.Error(w, "削除に失敗しました", http.StatusInternalServerError)
		return
	}
	deleteUpload(name)
	fmt.Fprintf(w, "✅ %s を削除しました", name)
}

func downloadHandler(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		http.Error(w, "ファイル名が指定されていません", http.StatusBadRequest)
		return
	}
	fullPath := filepath.Join("uploads", filepath.Clean("/"+name))
	if !strings.HasPrefix(fullPath, filepath.Clean("uploads")+string(os.PathSeparator)) {
		http.Error(w, "不正なパスです", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename="+filepath.Base(name))
	w.Header().Set("Content-Type", "application/zip")
	http.ServeFile(w, r, fullPath)
}

func toHostPath(containerPath string) string {
	if hostUploadDir == "" {
		return containerPath
	}
	const containerUploads = "/app/uploads"
	absPath, err := filepath.Abs(containerPath)
	if err != nil {
		return containerPath
	}
	if strings.HasPrefix(absPath, containerUploads) {
		rel := strings.TrimPrefix(absPath, containerUploads)
		return filepath.Join(hostUploadDir, rel)
	}
	abs2 := filepath.Join("/app", containerPath)
	if strings.HasPrefix(abs2, containerUploads) {
		rel := strings.TrimPrefix(abs2, containerUploads)
		return filepath.Join(hostUploadDir, rel)
	}
	return containerPath
}

func findPython() string {
	for _, name := range []string{"python3", "python"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return "python3"
}

func findPyrunner() string {
	candidates := []string{
		"/usr/local/bin/pyrunner.py",
		"pyrunner.py",
		"../pyrunner.py",
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "pyrunner.py"))
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
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
	if !requireTeacher(w, r.URL.Query().Get("operator")) {
		return
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		http.Error(w, "ファイル名が指定されていません", http.StatusBadRequest)
		return
	}
	zipPath := filepath.Join("uploads", filepath.Clean("/"+name))
	if !strings.HasPrefix(zipPath, filepath.Clean("uploads")+string(os.PathSeparator)) {
		http.Error(w, "不正なパスです", http.StatusBadRequest)
		return
	}
	tmpBase := filepath.Join("uploads", "tmp")
	os.MkdirAll(tmpBase, 0755)
	tmpDir, err := os.MkdirTemp(tmpBase, "pylab-*")
	if err != nil {
		http.Error(w, "一時フォルダの作成に失敗しました", http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(tmpDir)

	if err := unzip(zipPath, tmpDir); err != nil {
		http.Error(w, "zip解凍に失敗しました: "+err.Error(), http.StatusInternalServerError)
		return
	}

	projectToml := ""
	mainPy := ""
	filepath.Walk(tmpDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		switch info.Name() {
		case "project.toml":
			projectToml = path
		case "main.py":
			if mainPy == "" {
				mainPy = path
			}
		}
		return nil
	})

	timeoutSec := 30
	if v := os.Getenv("RUN_TIMEOUT_SEC"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			timeoutSec = n
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	useDocker := exec.Command("docker", "info").Run() == nil

	var cmd *exec.Cmd
	var debugInfo string

	if useDocker {
		imageExists := exec.Command("docker", "image", "inspect", "pylab-python").Run() == nil

		if projectToml != "" {
			codeDir := filepath.Dir(projectToml)
			hostCodeDir := toHostPath(codeDir)
			debugInfo = fmt.Sprintf("[docker] image=%v codeDir=%s hostCodeDir=%s", imageExists, codeDir, hostCodeDir)
			if imageExists {
				cmd = exec.CommandContext(ctx, "docker", "run", "--rm",
					"--memory", "256m",
					"-v", hostCodeDir+":/code",
					"pylab-python",
					"python3", "/usr/local/bin/pyrunner.py", "/code",
				)
			} else {
				pyrunner := filepath.Join(filepath.Dir(projectToml), "..", "..", "pyrunner.py")
				if _, err := os.Stat(pyrunner); os.IsNotExist(err) {
					pyrunner = "/usr/local/bin/pyrunner.py"
				}
				python3 := findPython()
				cmd = exec.CommandContext(ctx, python3, pyrunner, codeDir)
				debugInfo += " [fallback: direct python]"
			}
		} else if mainPy != "" {
			codeDir := filepath.Dir(mainPy)
			hostCodeDir := toHostPath(codeDir)
			debugInfo = fmt.Sprintf("[docker] image=%v mainPy=%s hostCodeDir=%s", imageExists, mainPy, hostCodeDir)
			if imageExists {
				cmd = exec.CommandContext(ctx, "docker", "run", "--rm",
					"--network", "none",
					"--memory", "128m",
					"-v", hostCodeDir+":/code",
					"pylab-python",
					"python3", "/code/main.py",
				)
			} else {
				python3 := findPython()
				cmd = exec.CommandContext(ctx, python3, mainPy)
				debugInfo += " [fallback: direct python]"
			}
		} else {
			http.Error(w, "main.py も project.toml も見つかりません", http.StatusBadRequest)
			return
		}
	} else {
		if projectToml != "" {
			codeDir := filepath.Dir(projectToml)
			pyrunner := findPyrunner()
			python3 := findPython()
			debugInfo = fmt.Sprintf("[no-docker] python=%s pyrunner=%s codeDir=%s", python3, pyrunner, codeDir)
			if pyrunner != "" {
				cmd = exec.CommandContext(ctx, python3, pyrunner, codeDir)
			} else {
				script := filepath.Join(codeDir, "main.py")
				cmd = exec.CommandContext(ctx, python3, script)
			}
		} else if mainPy != "" {
			python3 := findPython()
			debugInfo = fmt.Sprintf("[no-docker] python=%s mainPy=%s", python3, mainPy)
			cmd = exec.CommandContext(ctx, python3, mainPy)
		} else {
			http.Error(w, "main.py も project.toml も見つかりません", http.StatusBadRequest)
			return
		}
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	runErr := cmd.Run()

	stderr := errOut.String()
	if ctx.Err() == context.DeadlineExceeded {
		stderr += fmt.Sprintf("\n⏱ 実行がタイムアウトしました（%d秒）", timeoutSec)
	}
	if runErr != nil {
		stderr += fmt.Sprintf("\n[exit error] %v", runErr)
	}
	if debugInfo != "" {
		stderr += fmt.Sprintf("\n[debug] %s", debugInfo)
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
	// 課題作成（POST /api/tasks）と削除（DELETE /api/tasks/*）はteacherのみ許可
	// ロール管理はGoサーバー側のusers.jsonで行う
	apiPath := strings.TrimPrefix(r.URL.Path, "/api")
	if (r.Method == http.MethodPost && apiPath == "/tasks") ||
		(r.Method == http.MethodDelete && strings.HasPrefix(apiPath, "/tasks/")) {

		// POSTはJSONボディのcreated_by、DELETEはクエリのusernameで判断
		operator := r.URL.Query().Get("username")
		if r.Method == http.MethodPost {
			// ボディを読んでcreated_byを取得しつつ、後でプロキシに流せるよう復元
			bodyBytes, err := io.ReadAll(r.Body)
			r.Body.Close()
			if err != nil {
				http.Error(w, "リクエスト読み込みエラー", http.StatusBadRequest)
				return
			}
			var payload struct {
				CreatedBy string `json:"created_by"`
			}
			json.Unmarshal(bodyBytes, &payload)
			operator = payload.CreatedBy
			// ボディを復元
			r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			r.ContentLength = int64(len(bodyBytes))
		}

		if !requireTeacher(w, operator) {
			return
		}
	}

	target, err := url.Parse(taskServerURL)
	if err != nil {
		http.Error(w, "プロキシ設定エラー", http.StatusInternalServerError)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
	if r.URL.Path == "" {
		r.URL.Path = "/"
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, "タスクサーバーに接続できません: "+err.Error(), http.StatusBadGateway)
	}

	proxy.ServeHTTP(w, r)
}

func cleanupTmp() {
	tmpBase := filepath.Join("uploads", "tmp")
	entries, err := os.ReadDir(tmpBase)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-30 * time.Minute)
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			os.RemoveAll(filepath.Join(tmpBase, e.Name()))
		}
	}
}

func main() {
	initDB()
	os.MkdirAll("uploads", 0755)

	// 起動時にtmpの残骸を掃除
	go cleanupTmp()
	// 30分おきに定期クリーンアップ
	go func() {
		ticker := time.NewTicker(30 * time.Minute)
		for range ticker.C {
			cleanupTmp()
		}
	}()

	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	http.Handle("/api/", http.HandlerFunc(apiProxyHandler))
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
