package jmap

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// GET /jmap/download/{accountId}/{blobId}/{name}
func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authUser(w, r); !ok {
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/jmap/download/")
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 2 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	data, err := s.readBlob(parts[1])
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = w.Write(data)
}

// POST /jmap/upload/{accountId}/
func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authUser(w, r); !ok {
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(405)
		return
	}
	if s.BlobDir == "" {
		http.Error(w, "upload disabled", http.StatusNotImplemented)
		return
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, 50<<20))
	if err != nil {
		http.Error(w, "read failed", http.StatusBadRequest)
		return
	}
	id := "u" + randToken()
	if err := os.MkdirAll(s.BlobDir, 0o700); err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	if err := os.WriteFile(filepath.Join(s.BlobDir, id), data, 0o600); err != nil {
		http.Error(w, "write failed", http.StatusInternalServerError)
		return
	}
	// accountId 从路径 /jmap/upload/{accountId}/ 取
	acct := ""
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) >= 3 {
		acct = parts[2]
	}
	writeJSON(w, 201, map[string]any{
		"accountId": acct, "blobId": id,
		"type": r.Header.Get("Content-Type"), "size": len(data),
	})
}

func (s *Server) readUpload(blobID string) ([]byte, error) {
	if s.BlobDir == "" {
		return nil, fmt.Errorf("upload disabled")
	}
	return os.ReadFile(filepath.Join(s.BlobDir, filepath.Base(blobID)))
}

func randToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
