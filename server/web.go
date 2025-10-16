package server

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Server represents the web server which serves a small upload page and accepts files.
type Server struct {
	Addr      string
	InputDir  string
	OutputDir string
	log       *logrus.Entry
	srv       *http.Server
}

// New creates a new Server. addr is the listen address (e.g., ":8080").
func New(addr string, inputDir string, outputDir string, logger *logrus.Logger) *Server {
	return &Server{
		Addr:      addr,
		InputDir:  inputDir,
		OutputDir: outputDir,
		log:       logger.WithField("component", "webserver"),
	}
}

func (s *Server) uploadPageHandler(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles(filepath.Join("server", "resources", "upload.html"))
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		s.log.Errorf("template parse error: %v", err)
		return
	}
	data := map[string]string{"LogoPath": "/resources/cassecroutefinal.png"}
	if err := tmpl.Execute(w, data); err != nil {
		s.log.Errorf("template execute error: %v", err)
	}
}

func (s *Server) resourceHandler() http.Handler {
	fs := http.FileServer(http.Dir(filepath.Join("server", "resources")))
	return http.StripPrefix("/resources/", fs)
}

// streamHandler streams the contents of the debug file as server-sent events.
func (s *Server) streamHandler(w http.ResponseWriter, r *http.Request) {
	file := r.URL.Query().Get("file")
	if file == "" {
		http.Error(w, "missing file parameter", http.StatusBadRequest)
		return
	}
	// sanitize
	file = filepath.Base(file)
	path := filepath.Join(s.OutputDir, file)

	// ensure file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// create empty file so client can still connect
		_ = ioutil.WriteFile(path, []byte(""), 0644)
	}

	f, err := os.Open(path)
	if err != nil {
		http.Error(w, "unable to open file", http.StatusInternalServerError)
		s.log.Errorf("open debug file error: %v", err)
		return
	}
	defer f.Close()

	// SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	// send initial content and then tail
	offset := int64(0)
	for {
		select {
		case <-r.Context().Done():
			return
		default:
			// stat
			fi, err := os.Stat(path)
			if err != nil {
				time.Sleep(500 * time.Millisecond)
				continue
			}
			size := fi.Size()
			if size > offset {
				// read new bytes
				f.Seek(offset, io.SeekStart)
				data, _ := ioutil.ReadAll(f)
				offset = size
				// send as SSE, prefix each line
				lines := string(data)
				for _, l := range splitLines(lines) {
					fmt.Fprintf(w, "data: %s\n", l)
				}
				fmt.Fprint(w, "\n")
				flusher.Flush()
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
}

// existsHandler returns JSON {"exists": true|false} depending on whether the
// given file exists in the output directory. Used by the client to detect when
// the debug file has been removed by the processor.
func (s *Server) existsHandler(w http.ResponseWriter, r *http.Request) {
	file := r.URL.Query().Get("file")
	if file == "" {
		http.Error(w, "missing file parameter", http.StatusBadRequest)
		return
	}
	file = filepath.Base(file)
	// the client passes the debug filename (e.g. 163000_name.pdf.debug.txt)
	// the processor now writes a marker <basename>.success where basename is
	// the original output filename (i.e. debug with ".debug.txt" removed).
	// derive the success filename and check for its existence.
	successName := file
	if len(successName) > 10 && successName[len(successName)-10:] == ".debug.txt" {
		successName = successName[:len(successName)-10]
	}
	successName = successName + ".success"
	path := filepath.Join(s.OutputDir, successName)
	_, err := os.Stat(path)
	exists := err == nil

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"exists": exists})
}

// downloadHandler serves a produced output file from the output directory as
// an attachment. The client will call this when processing is finished.
func (s *Server) downloadHandler(w http.ResponseWriter, r *http.Request) {
	file := r.URL.Query().Get("file")
	if file == "" {
		http.Error(w, "missing file parameter", http.StatusBadRequest)
		return
	}
	file = filepath.Base(file)
	path := filepath.Join(s.OutputDir, file)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename=\""+file+"\"")
	http.ServeFile(w, r, path)
}

func splitLines(s string) []string {
	// simple split preserving empty final line
	var out []string
	if s == "" {
		return []string{""}
	}
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start <= len(s)-1 {
		out = append(out, s[start:])
	} else if start == len(s) {
		// trailing newline -> add empty line so client sees newline
		out = append(out, "")
	}
	return out
}

func (s *Server) uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Limit size to 100MB to avoid abuse
	r.Body = http.MaxBytesReader(w, r.Body, 100<<20)
	if err := r.ParseMultipartForm(100 << 20); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		s.log.Errorf("parse multipart form error: %v", err)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Missing file", http.StatusBadRequest)
		s.log.Errorf("form file error: %v", err)
		return
	}
	defer file.Close()

	// Ensure input dir exists
	if err := os.MkdirAll(s.InputDir, 0755); err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		s.log.Errorf("mkdir input dir: %v", err)
		return
	}

	// Use a timestamp prefix to avoid clashing names
	dstName := fmt.Sprintf("%d_%s", time.Now().Unix(), filepath.Base(header.Filename))
	dstPath := filepath.Join(s.InputDir, dstName)

	dst, err := os.Create(dstPath)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		s.log.Errorf("create file error: %v", err)
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		s.log.Errorf("copy file error: %v", err)
		return
	}

	s.log.Infof("Saved uploaded file to %s", dstPath)
	// create a debug file in OutputDir named <originalname>.debug.txt
	debugName := dstName + ".debug.txt"
	debugPath := filepath.Join(s.OutputDir, debugName)
	if err := os.MkdirAll(s.OutputDir, 0755); err == nil {
		_ = ioutil.WriteFile(debugPath, []byte(fmt.Sprintf("Debug for %s\nUploaded to: %s\n", header.Filename, dstPath)), 0644)
	} else {
		s.log.Errorf("could not create output dir: %v", err)
	}

	// Redirect to main page with debug file query so the page can open the stream
	redirect := "/?debug=" + url.QueryEscape(debugName)
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

// Start runs the HTTP server in background. Returns when server is listening.
func (s *Server) Start(wg *sync.WaitGroup) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.uploadPageHandler)
	mux.Handle("/resources/", s.resourceHandler())
	mux.HandleFunc("/upload", s.uploadHandler)
	mux.HandleFunc("/stream", s.streamHandler)
	mux.HandleFunc("/exists", s.existsHandler)
	mux.HandleFunc("/download", s.downloadHandler)

	s.srv = &http.Server{
		Addr:    s.Addr,
		Handler: mux,
	}

	// Start server in goroutine and register with wait group
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.log.Infof("Starting webserver on %s", s.Addr)
		if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.log.Errorf("webserver error: %v", err)
		}
	}()
	return nil
}

// Shutdown gracefully stops the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.srv == nil {
		return nil
	}
	s.log.Info("Shutting down webserver")
	return s.srv.Shutdown(ctx)
}

// (no additional types)
