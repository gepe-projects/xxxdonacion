package config

import "testing"

func validApp(env string) *App {
	return &App{
		Environment: env,
		Name:        "app",
		Port:        8080,
		Database: Database{
			URL: "postgres://root:root@localhost:5432/app?sslmode=disable",
		},
		Redis:    Redis{Addr: "localhost:6379", Password: "root"},
		Firebase: Firebase{CredentialsFile: "secrets/service-account.json"},
	}
}

func TestValidateDevelopmentAllowsLocalDefaults(t *testing.T) {
	if err := validApp("development").validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateTestEnvironmentAllowsLocalDefaults(t *testing.T) {
	if err := validApp("test").validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateStagingRejectsDefaultCredentials(t *testing.T) {
	if err := validApp("staging").validate(); err == nil {
		t.Fatal("expected staging validation error")
	}
}

func TestValidateRequiresEnvironment(t *testing.T) {
	app := validApp("")
	if err := app.validate(); err == nil {
		t.Fatal("expected APP_ENV error")
	}
}

func TestValidateProductionRejectsDefaultCredentials(t *testing.T) {
	tests := []struct {
		name string
		app  *App
	}{
		{
			name: "root database",
			app:  validApp("production"),
		},
		{
			name: "root redis",
			app: func() *App {
				app := validApp("production")
				app.Database.URL = "postgres://app:strong-password@db:5432/app?sslmode=disable"
				return app
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.app.validate(); err == nil {
				t.Fatal("expected production validation error")
			}
		})
	}
}

func TestValidateProductionAcceptsPrivateComposeConfig(t *testing.T) {
	app := validApp("production")
	app.Database.URL = "postgres://app:strong-password@db:5432/app?sslmode=disable"
	app.Redis.Password = "strong-redis-password"
	app.CORSOrigins = []string{"https://app.example.com"}

	if err := app.validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateProductionRejectsWildcardCORS(t *testing.T) {
	app := validApp("production")
	app.Database.URL = "postgres://app:strong-password@db:5432/app?sslmode=disable"
	app.Redis.Password = "strong-redis-password"
	app.CORSOrigins = []string{"*"}

	if err := app.validate(); err == nil {
		t.Fatal("expected wildcard CORS error")
	}
}
