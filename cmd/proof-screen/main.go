package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	proofscreen "github.com/infrai-examples/logistics-proof-screen"
	"github.com/openai/openai-go"
)

type handler struct {
	screener *proofscreen.Screener
}

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	mux := http.NewServeMux()
	mux.Handle("POST /shipments/{shipmentID}/proof", handler{proofscreen.NewScreener(proofscreen.NewInfraiClient(key))})
	log.Printf("proof screening service listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(12 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart request"})
		return
	}
	file, header, err := r.FormFile("image")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "image is required"})
		return
	}
	defer file.Close()
	image, err := io.ReadAll(io.LimitReader(file, 10<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "could not read image"})
		return
	}

	event := proofscreen.ShipmentEvent{
		ShipmentID: r.PathValue("shipmentID"), EventType: r.FormValue("event_type"),
		Caption: r.FormValue("caption"), Filename: header.Filename, Image: image,
	}
	requestID := r.Header.Get("Idempotency-Key")
	if requestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Idempotency-Key is required"})
		return
	}
	proof, err := h.screener.Screen(r.Context(), event, requestID)
	if err != nil {
		writeJSON(w, statusFor(err), map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, proof)
}

func statusFor(err error) int {
	var infraiErr *proofscreen.InfraiError
	if errors.As(err, &infraiErr) && infraiErr.Status >= 400 && infraiErr.Status < 500 {
		return infraiErr.Status
	}
	var openaiErr *openai.Error
	if errors.As(err, &openaiErr) && openaiErr.StatusCode >= 400 && openaiErr.StatusCode < 500 {
		return openaiErr.StatusCode
	}
	if strings.Contains(err.Error(), "required") {
		return http.StatusBadRequest
	}
	return http.StatusBadGateway
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
