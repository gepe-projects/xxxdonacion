package identity

import (
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v5"
	"github.com/redis/go-redis/v9"

	"xxxdonacion/modules/identity/api/domain"
	"xxxdonacion/modules/identity/internal/delivery/http"
	"xxxdonacion/modules/identity/internal/service"
	"xxxdonacion/platform/firebase"
	"xxxdonacion/platform/server/middleware"
)

type Dependencies struct {
	DB       *pgxpool.Pool
	Redis    *redis.Client
	Firebase *firebase.Client
}

type Module struct {
	deps Dependencies
	svc  *service.Service
}

func New(deps Dependencies) *Module {
	return &Module{
		deps: deps,
		svc:  service.New(deps.DB, deps.Redis, deps.Firebase.AuthClient()),
	}
}

const (
	rateLimitWindow = time.Minute
	syncRateLimit   = 10
	adminRateLimit  = 60
)

func (m *Module) RegisterRoutes(e *echo.Echo) {
	h := http.New(m.svc)

	user := e.Group("identities", middleware.RequireAuth(m.deps.Firebase))
	user.POST("/sync", h.Sync, middleware.RateLimit(
		m.deps.Redis, "identity:sync", syncRateLimit, rateLimitWindow, middleware.IdentityKey,
	))
	user.GET("/me", h.Me)

	// Path admin dipisah dari endpoint user. Otorisasi tetap dicek di service,
	// bukan hanya mengandalkan prefix route. Admin memakai pengecekan
	// revocation karena aksinya sensitif.
	admin := e.Group("identities/admin",
		middleware.RequireAuthRevoked(m.deps.Firebase),
		middleware.RateLimit(m.deps.Redis, "identity:admin", adminRateLimit, rateLimitWindow, middleware.IdentityKey),
	)
	admin.POST("/users", h.CreateUser)
	admin.GET("/users", h.ListUsers)
	admin.PATCH("/users/:id/status", h.SetUserStatus)
	admin.PUT("/users/:id/roles", h.SetUserRoles)
	admin.DELETE("/users/:id", h.DeleteUser)
}

// Exposed buat module lain yang butuh.
func (m *Module) Services() domain.Service { return m.svc }
