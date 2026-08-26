package idempotency

import "net/http"

// recorder wraps an http.ResponseWriter, capturing the status code and body
// written by the downstream handler while still passing them through to the
// real client, so the response can be persisted for replay on retry.
type recorder struct {
	http.ResponseWriter
	status int
	body   []byte
}

func newRecorder(w http.ResponseWriter) *recorder {
	return &recorder{ResponseWriter: w, status: http.StatusOK}
}

func (r *recorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *recorder) Write(b []byte) (int, error) {
	r.body = append(r.body, b...)
	return r.ResponseWriter.Write(b)
}
