// Package server defines the HTTP requests Sammy can handle.
package server

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Kerliula/sammy-memory/internal/retain"
)

type Server struct {
	database *pgxpool.Pool
	retainer *retain.Service
}

func New(database *pgxpool.Pool, retainer *retain.Service) http.Handler {
	router := http.NewServeMux()

	apiSettings := huma.DefaultConfig("Sammy", "0.1.0")
	api := humago.New(router, apiSettings)
	server := &Server{database: database, retainer: retainer}

	healthRoute := huma.Operation{
		OperationID: "healthz",
		Method:      http.MethodGet,
		Path:        "/healthz",
		Summary:     "Health check",
	}

	huma.Register(api, healthRoute, server.checkHealth)
	huma.Register(api, retainRoute, server.retainMessages)

	return router
}

type healthOutput struct {
	Body struct {
		Status string `json:"status" example:"ok"`
	}
}

func (server *Server) checkHealth(ctx context.Context, _ *struct{}) (*healthOutput, error) {
	err := server.database.Ping(ctx)

	if err != nil {
		return nil, huma.Error503ServiceUnavailable("database unavailable")
	}

	response := &healthOutput{}
	response.Body.Status = "ok"

	return response, nil
}
