package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/middleware"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/service"
)

type Server struct {
	service         *service.Service
	sessionTTL      time.Duration
	maxRequestBytes int64
	logger          *slog.Logger
}

func New(svc *service.Service, sessionTTL time.Duration, maxRequestBytes int64, logger *slog.Logger) http.Handler {
	server := &Server{service: svc, sessionTTL: sessionTTL, maxRequestBytes: maxRequestBytes, logger: logger}
	public := http.NewServeMux()
	public.HandleFunc("GET /healthz", server.health)
	public.HandleFunc("GET /readyz", server.ready)
	public.HandleFunc("POST /v1/auth/login", server.login)
	protected := http.NewServeMux()
	protected.HandleFunc("POST /v1/auth/logout", server.logout)
	protected.HandleFunc("GET /v1/projects", server.listProjects)
	protected.HandleFunc("POST /v1/projects", server.createProject)
	protected.HandleFunc("GET /v1/projects/{projectID}", server.getProject)
	protected.HandleFunc("POST /v1/projects/{projectID}/work-packages", server.createWork)
	protected.HandleFunc("POST /v1/work-packages/{workID}/transitions", server.transitionWork)
	protected.HandleFunc("POST /v1/work-packages/{workID}/inspections", server.scheduleInspection)
	protected.HandleFunc("POST /v1/inspections/{inspectionID}/complete", server.completeInspection)
	protected.HandleFunc("POST /v1/findings/{findingID}/resolve", server.resolveFinding)
	protected.HandleFunc("POST /v1/projects/{projectID}/load-plans", server.createLoadPlan)
	protected.HandleFunc("POST /v1/projects/{projectID}/load-plans/{planID}/approve", server.approveLoadPlan)
	protected.HandleFunc("POST /v1/projects/{projectID}/load-plans/{planID}/runs", server.startLoadRun)
	protected.HandleFunc("POST /v1/load-runs/{runID}/readings", server.appendReading)
	protected.HandleFunc("POST /v1/load-runs/{runID}/evaluate", server.queueEvaluation)
	protected.HandleFunc("POST /v1/projects/{projectID}/dossiers", server.createDossier)
	protected.HandleFunc("PUT /v1/projects/{projectID}/dossiers/{dossierID}/documents/{kind}", server.receiveDossierDocument)
	protected.HandleFunc("POST /v1/projects/{projectID}/dossiers/{dossierID}/submit", server.submitDossier)
	protected.HandleFunc("POST /v1/projects/{projectID}/dossiers/{dossierID}/decision", server.decideDossier)
	protected.HandleFunc("GET /v1/projects/{projectID}/opening-readiness", server.readiness)
	public.Handle("/v1/", middleware.RequireAuth(svc, server.writeError, protected))
	return middleware.Recover(logger, server.writeError, middleware.RequestIDs(middleware.Logging(logger, public)))
}
