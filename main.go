package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"image"
	"image/color"
	"image/jpeg"
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
	ImagePath      string
	ModifiedImages []string
	Zsteg          string
	Strings        string
	ExifData       string
	BinwalkData    string
	Done           bool
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

func adjustImageBrightness(img image.Image, brightnessFactor float64) image.Image {
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	adjustedImage := image.NewRGBA(bounds)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			adjustedR := uint8(float64(r>>8) * brightnessFactor)
			adjustedG := uint8(float64(g>>8) * brightnessFactor)
			adjustedB := uint8(float64(b>>8) * brightnessFactor)
			adjustedImage.Set(x, y, color.RGBA{adjustedR, adjustedG, adjustedB, uint8(a >> 8)})
		}
	}

	return adjustedImage
}

func adjustImageContrast(img image.Image, contrastFactor float64) image.Image {
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	adjustedImage := image.NewRGBA(bounds)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			adjustedR := uint8(float64(r>>8) * contrastFactor)
			adjustedG := uint8(float64(g>>8) * contrastFactor)
			adjustedB := uint8(float64(b>>8) * contrastFactor)
			adjustedImage.Set(x, y, color.RGBA{adjustedR, adjustedG, adjustedB, uint8(a >> 8)})
		}
	}

	return adjustedImage
}

func adjustImageGamma(img image.Image, gammaFactor float64) image.Image {
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	adjustedImage := image.NewRGBA(bounds)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			adjustedR := uint8(float64(r>>8) / gammaFactor)
			adjustedG := uint8(float64(g>>8) / gammaFactor)
			adjustedB := uint8(float64(b>>8) / gammaFactor)
			adjustedImage.Set(x, y, color.RGBA{adjustedR, adjustedG, adjustedB, uint8(a >> 8)})
		}
	}

	return adjustedImage
}

func adjustImageColors(img image.Image, redFactor, greenFactor, blueFactor float64) image.Image {
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	adjustedImage := image.NewRGBA(bounds)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			adjustedR := uint8(float64(r>>8) * redFactor)
			adjustedG := uint8(float64(g>>8) * greenFactor)
			adjustedB := uint8(float64(b>>8) * blueFactor)
			adjustedImage.Set(x, y, color.RGBA{adjustedR, adjustedG, adjustedB, uint8(a >> 8)})
		}
	}

	return adjustedImage
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

		go processAndAnalyze(taskID, filePath)

		response := map[string]string{"task_id": taskID}
		json.NewEncoder(w).Encode(response)
	}
}

func processAndAnalyze(taskID, filePath string) {
	var wg sync.WaitGroup
	task := tasks[taskID]

	modifiedImages := []string{}
	brightness := 2.5
	contrast := 2.5
	gamma := 0.5
	redFactor := 2.0
	greenFactor := 1.5
	blueFactor := 1.0

	imgFile, err := os.Open(filePath)
	if err != nil {
		log.Fatal(err)
	}
	defer imgFile.Close()

	img, _, err := image.Decode(imgFile)
	if err != nil {
		log.Fatal(err)
	}

	brightnessFile := filepath.Join(uploadDir, fmt.Sprintf("%s_brightness.jpg", taskID))
	brightnessImage := adjustImageBrightness(img, brightness)
	output, err := os.Create(brightnessFile)
	if err != nil {
		log.Fatal(err)
	}
	defer output.Close()

	err = jpeg.Encode(output, brightnessImage, nil)
	if err != nil {
		log.Fatal(err)
	}
	modifiedImages = append(modifiedImages, brightnessFile)

	contrastFile := filepath.Join(uploadDir, fmt.Sprintf("%s_contrast.jpg", taskID))
	contrastImage := adjustImageContrast(img, contrast)
	output, err = os.Create(contrastFile)
	if err != nil {
		log.Fatal(err)
	}
	defer output.Close()

	err = jpeg.Encode(output, contrastImage, nil)
	if err != nil {
		log.Fatal(err)
	}
	modifiedImages = append(modifiedImages, contrastFile)

	gammaFile := filepath.Join(uploadDir, fmt.Sprintf("%s_gamma.jpg", taskID))
	gammaImage := adjustImageGamma(img, gamma)
	output, err = os.Create(gammaFile)
	if err != nil {
		log.Fatal(err)
	}
	defer output.Close()

	err = jpeg.Encode(output, gammaImage, nil)
	if err != nil {
		log.Fatal(err)
	}
	modifiedImages = append(modifiedImages, gammaFile)

	colorFile := filepath.Join(uploadDir, fmt.Sprintf("%s_color.jpg", taskID))
	colorImage := adjustImageColors(img, redFactor, greenFactor, blueFactor)
	output, err = os.Create(colorFile)
	if err != nil {
		log.Fatal(err)
	}
	defer output.Close()

	err = jpeg.Encode(output, colorImage, nil)
	if err != nil {
		log.Fatal(err)
	}
	modifiedImages = append(modifiedImages, colorFile)

	wg.Add(5)

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
	go func() {
		defer wg.Done()
		task.Zsteg = runCmd("zsteg", "-a", brightnessFile)
	}()

	wg.Wait()

	mu.Lock()
	task.Done = true
	task.ModifiedImages = modifiedImages
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
	log.Fatal(http.ListenAndServe(":8082", nil))
}
