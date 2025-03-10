package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

const uploadDir = "./uploads"

type AnalysisResult struct {
	ImagePath   string
	Zsteg       string
	Strings     string
	ExifData    string
	BinwalkData string
	Done        bool
}

var (
	mu    sync.Mutex
	tasks = make(map[string]*AnalysisResult)
)

var templates = template.Must(template.ParseFiles("templates/upload.html", "templates/result.html"))

func runCmd(tool string, args ...string) string {
	cmd := exec.Command(tool, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("Ошибка (%s): %v", tool, err)
	}
	return string(output)
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		templates.ExecuteTemplate(w, "upload.html", nil)
		return
	}

	if r.Method == "POST" {
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "Ошибка загрузки файла", http.StatusBadRequest)
			return
		}
		defer file.Close()

		if _, err := os.Stat(uploadDir); os.IsNotExist(err) {
			os.Mkdir(uploadDir, os.ModePerm)
		}

		filePath := filepath.Join(uploadDir, header.Filename)
		dst, err := os.Create(filePath)
		if err != nil {
			http.Error(w, "Ошибка сохранения файла", http.StatusInternalServerError)
			return
		}
		defer dst.Close()
		io.Copy(dst, file)

		taskID := fmt.Sprintf("%d", rand.Intn(100000))
		mu.Lock()
		tasks[taskID] = &AnalysisResult{ImagePath: filePath}
		mu.Unlock()

		go analyzeImage(taskID, filePath)

		// Отправляем JSON вместо редиректа
		response := map[string]string{"task_id": taskID}
		json.NewEncoder(w).Encode(response)
	}
}

func analyzeImage(taskID, filePath string) {
	var wg sync.WaitGroup
	task := tasks[taskID]

	wg.Add(4)

	go func() {
		defer wg.Done()
		task.Zsteg = runCmd("zsteg", "-a", filePath)
	}()
	go func() {
		defer wg.Done()
		task.Strings = runCmd("strings", filePath)
	}()
	go func() {
		defer wg.Done()
		task.ExifData = runCmd("exiv2", filePath)
	}()
	go func() {
		defer wg.Done()
		task.BinwalkData = runCmd("binwalk", filePath)
	}()

	wg.Wait()
	mu.Lock()
	task.Done = true
	mu.Unlock()
}

func resultHandler(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("task_id")
	if taskID == "" {
		http.Error(w, "Не указан task_id", http.StatusBadRequest)
		return
	}

	templates.ExecuteTemplate(w, "result.html", map[string]string{
		"TaskID": taskID,
	})
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("task_id")

	mu.Lock()
	task, exists := tasks[taskID]
	mu.Unlock()

	if !exists {
		http.Error(w, "Задача не найдена", http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(task)
}

func main() {
	rand.Seed(time.Now().UnixNano())

	http.HandleFunc("/", uploadHandler)
	http.HandleFunc("/upload", uploadHandler)
	http.HandleFunc("/result", resultHandler)
	http.HandleFunc("/status", statusHandler)

	fs := http.FileServer(http.Dir(uploadDir))
	http.Handle("/uploads/", http.StripPrefix("/uploads/", fs))

	fmt.Println("Сервер запущен на http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
