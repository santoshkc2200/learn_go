package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"unicode/utf8"

	"example.com/tasksvc/internal/tasks"
)

const maxBodyBytes = 16 * 1024

type createRequest struct {
	Title *string `json:"title"`
}

func decodeCreate(w http.ResponseWriter, r *http.Request) (string, int, error) {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return "", 415, errors.New("Content-Type must be application/json")
	}
	// Read the WHOLE bounded body: a decoder alone could miss oversized trailing space.
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return "", 413, errors.New("request body exceeds 16 KiB")
		}
		return "", 400, errors.New("cannot read request body")
	}
	if !utf8.Valid(body) {
		return "", 400, errors.New("request body must be valid UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var input *createRequest
	if err := decoder.Decode(&input); err != nil || input == nil {
		return "", 400, errors.New("invalid JSON object")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return "", 400, errors.New("expected one JSON value")
	}
	// A pointer makes a final null overwrite a prior duplicate string value.
	if input.Title == nil {
		return "", 400, errors.New("title must contain 1–200 Unicode code points")
	}
	title, err := tasks.NormalizeTitle(*input.Title)
	if err != nil {
		return "", 400, errors.New("title must contain 1–200 Unicode code points")
	}
	return title, 0, nil
}

func (a *api) writeError(w http.ResponseWriter, status int, message string) {
	a.writeJSON(w, status, struct {
		Error string `json:"error"`
	}{message})
}

func (a *api) writeJSON(w http.ResponseWriter, status int, value any) {
	// Marshal before headers; after WriteHeader it is too late to change status.
	body, err := json.Marshal(value)
	if err != nil {
		a.logger.Error("encode response", "error", err)
		status = 500
		body = []byte(`{"error":"internal server error"}`)
	}
	body = append(body, '\n')
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	n, err := w.Write(body)
	if err == nil && n != len(body) {
		err = io.ErrShortWrite
	}
	if err != nil {
		a.logger.Error("write response", "error", err)
	} // never write a second response
}
