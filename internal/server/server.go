// Package server wires Sammy's HTTP API: net/http + huma v2.
package server

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Server holds the dependencies the HTTP handlers need.
type Server struct {
	pool *pgxpool.Pool
}

// New returns an http.Handler serving the huma API (including the
// OpenAPI 3.1 spec and docs huma mounts from the config) on a stdlib mux.
func New(pool *pgxpool.Pool) http.Handler {
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("Sammy", "0.1.0"))
	s := &Server{pool: pool}

	huma.Register(api, huma.Operation{
		OperationID: "healthz",
		Method:      http.MethodGet,
		Path:        "/healthz",
		Summary:     "Health check",
	}, s.healthz)

	return mux
}

type healthOutput struct {
	Body struct {
		Status string `json:"status" example:"ok"`
	}
}

func (s *Server) healthz(ctx context.Context, _ *struct{}) (*healthOutput, error) {
	if err := s.pool.Ping(ctx); err != nil {
		return nil, huma.Error503ServiceUnavailable("database unavailable")
	}
	out := &healthOutput{}
	out.Body.Status = "ok"
	return out, nil
}
