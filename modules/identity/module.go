package identity

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v5"
	"github.com/redis/go-redis/v9"

	"xxxdonacion/modules/identity/api/domain"
	"xxxdonacion/modules/identity/internal/delivery/http"
	"xxxdonacion/modules/identity/internal/service"
)

type Dependencies struct {
	DB    *pgxpool.Pool
	Redis *redis.Client
}

type Module struct {
	deps Dependencies
	svc  *service.Service
}

func New(deps Dependencies) *Module {
	return &Module{
		deps: deps,
		svc:  service.New(deps.DB, deps.Redis),
	}
}

func (m *Module) RegisterRoutes(e *echo.Echo) {
	h := http.New(m.svc)

	g := e.Group("identities")

	g.GET("/foo", h.Foo)
	g.GET("/internal", h.InternalError)
}

// exposed buat module lain yang butuh
func (m *Module) Services() domain.Service { return m.svc }
