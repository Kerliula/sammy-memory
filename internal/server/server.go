// Package server defines the HTTP requests Sammy can handle.
package server

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
	database *pgxpool.Pool
}

func New(database *pgxpool.Pool) http.Handler {
	router := http.NewServeMux()

	apiSettings := huma.DefaultConfig("Sammy", "0.1.0")
	api := humago.New(router, apiSettings)
	server := &Server{database: database}

	healthRoute := huma.Operation{
		OperationID: "healthz",
		Method:      http.MethodGet,
		Path:        "/healthz",
		Summary:     "Health check",
	}

	huma.Register(api, healthRoute, server.checkHealth)

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
