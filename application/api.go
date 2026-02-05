package application

import (
	"log"
	"net/http"
	"time"

	"github.com/freitasmatheusrn/lifecycle-monitor/assets"
	configs "github.com/freitasmatheusrn/lifecycle-monitor/configs"
	"github.com/freitasmatheusrn/lifecycle-monitor/internal/areas"
	authPkg "github.com/freitasmatheusrn/lifecycle-monitor/internal/auth"
	repo "github.com/freitasmatheusrn/lifecycle-monitor/internal/database/postgres/sqlc"
	redisdb "github.com/freitasmatheusrn/lifecycle-monitor/internal/database/redis"
	"github.com/freitasmatheusrn/lifecycle-monitor/internal/email/smtp"
	"github.com/freitasmatheusrn/lifecycle-monitor/internal/pages"
	"github.com/freitasmatheusrn/lifecycle-monitor/internal/products"
	"github.com/freitasmatheusrn/lifecycle-monitor/internal/scheduler"
	"github.com/freitasmatheusrn/lifecycle-monitor/internal/user"
	"github.com/freitasmatheusrn/lifecycle-monitor/pkg/auth"
	"github.com/freitasmatheusrn/lifecycle-monitor/pkg/parser"
	"github.com/freitasmatheusrn/lifecycle-monitor/pkg/rest"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	echojwt "github.com/labstack/echo-jwt/v4"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"
)

type Application struct {
	Config configs.Configs
	Logger *zap.Logger
	DB     *pgx.Conn
	Redis  *redisdb.Client
}

func (app *Application) Mount() http.Handler {
	email := smtp.New(
		app.Config.SMTP_HOST,
		app.Config.SMTP_USER,
		app.Config.SMTP_PASS,
		app.Config.SMTP_PORT,
	)
	e := echo.New()
	e.StaticFS("/assets", assets.Files)
	e.HTTPErrorHandler = app.CustomErrorHandler
	e.Use(middleware.Recover())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"http://localhost:3000"},
		AllowMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodDelete,
			http.MethodOptions,
		},
		AllowHeaders: []string{
			echo.HeaderOrigin,
			echo.HeaderContentType,
			echo.HeaderAccept,
			echo.HeaderAuthorization,
		},
		AllowCredentials: true,
	}))
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogLatency:  true,
		LogStatus:   true,
		LogURI:      true,
		LogMethod:   true,
		LogError:    true,
		HandleError: true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {

			status := v.Status
			if v.Error != nil {
				switch err := v.Error.(type) {
				case *echo.HTTPError:
					status = err.Code
				case *rest.ApiErr:
					status = err.Code
				}
			}

			if status >= 500 {
				app.Logger.Error("request",
					zap.Duration("latency", v.Latency),
					zap.Int("status", status),
					zap.String("uri", v.URI),
					zap.String("method", v.Method),
				)
				return nil
			}

			if status >= 400 {
				app.Logger.Warn("request",
					zap.Duration("latency", v.Latency),
					zap.Int("status", status),
					zap.String("uri", v.URI),
					zap.String("method", v.Method),
				)
				return nil
			}

			app.Logger.Info("request",
				zap.Duration("latency", v.Latency),
				zap.Int("status", status),
				zap.String("uri", v.URI),
				zap.String("method", v.Method),
			)
			return nil
		},
	}))
	config := echojwt.Config{
		NewClaimsFunc: func(c echo.Context) jwt.Claims {
			return new(auth.JWTCustomClaims)
		},
		SigningKey:  []byte(app.Config.JWTSecret),
		TokenLookup: "header:Authorization,cookie:access_token",
		SuccessHandler: func(c echo.Context) {
			usr := c.Get("user").(*jwt.Token)
			claims := usr.Claims.(*auth.JWTCustomClaims)
			pgUUID, err := parser.PgUUIDFromString(claims.UserID)
			if err != nil {
				_ = c.NoContent(http.StatusUnauthorized)
				return
			}
			currentUser := user.CurrentUser{
				ID:    pgUUID,
				Email: claims.Email,
			}

			user.SetCurrentUser(c, currentUser)
		},
	}
	// Initialize repositories and services
	querier := repo.New(app.DB)
	tokenRepo := authPkg.NewTokenRepository(app.Redis.Client)

	// Initialize auth service
	authService := authPkg.NewService(
		querier,
		tokenRepo,
		app.Config.JWTSecret,
		app.Config.AccessTokenExp,
		app.Config.RefreshTokenExp,
	)

	userService := user.NewService(querier)
	userHandler := user.NewHandler(userService)

	// Initialize auth handler
	authHandler := authPkg.NewHandler(authService, app.Config.AccessTokenExp, app.Config.RefreshTokenExp)

	// Initialize products crawler and worker pool
	crawler, err := products.NewCrawler(app.Config.SIEMENS_URL)
	if err != nil {
		app.Logger.Fatal("failed to create crawler", zap.Error(err))
	}

	workerPool := products.NewWorkerPool(crawler, querier, app.Logger, products.WorkerPoolConfig{
		NumWorkers: 5,
		QueueSize:  100,
	})

	if err := workerPool.Start(); err != nil {
		app.Logger.Fatal("failed to start worker pool", zap.Error(err))
	}

	// Initialize products service and handler
	productService := products.NewService(querier, workerPool, app.Config.SIEMENS_URL, app.Logger)
	productHandler := products.NewHandler(productService)

	// Initialize and start scheduler for lifecycle updates
	lifecycleScheduler := scheduler.NewScheduler(workerPool, productService, app.Logger, email, app.Config.AlertRecipients)
	if err := lifecycleScheduler.Start(app.Config.CronExpression); err != nil {
		app.Logger.Fatal("failed to start scheduler", zap.Error(err))
	}

	// Initialize areas service and handler
	areaService := areas.NewService(querier)
	areaHandler := areas.NewHandler(areaService)

	// Initialize pages handler
	pageHandler := pages.NewHandler(productService)

	// Public page routes (HTML)
	e.GET("/login", pageHandler.Login)
	e.GET("/signup", pageHandler.Signup)

	// Public API routes
	e.POST("/signup", authHandler.Signup)
	e.POST("/signin", authHandler.Signin)
	e.POST("/refresh", authHandler.Refresh)

	// Protected routes (JWT required)
	protected := e.Group("")
	protected.Use(authPkg.AutoRefreshMiddleware(authService, app.Config.AccessTokenExp, app.Config.RefreshTokenExp, app.Config.JWTSecret))
	protected.Use(echojwt.WithConfig(config))

	// Protected page routes
	protected.GET("/", pageHandler.Index)
	protected.GET("/products/form", pageHandler.ProductForm)

	// Protected API routes
	protected.POST("/logout", authHandler.Logout)
	protected.POST("/logout-all", authHandler.LogoutAll)
	protected.GET("/me", userHandler.GetMe)
	protected.PUT("/user", userHandler.Update)

	// Areas API routes
	protected.POST("/areas", areaHandler.CreateArea)
	protected.GET("/areas", areaHandler.ListAreas)
	protected.GET("/areas/:id", areaHandler.GetArea)
	protected.PUT("/areas/:id", areaHandler.UpdateArea)
	protected.DELETE("/areas/:id", areaHandler.DeleteArea)

	// Product API routes
	protected.POST("/products", productHandler.CreateProduct)
	protected.GET("/products", productHandler.ListProducts)
	protected.POST("/products/import", productHandler.ImportSpreadsheet)
	protected.POST("/products/import-stream", productHandler.ImportSpreadsheetSSE)
	protected.GET("/products/add-stream", productHandler.AddProductsSSE)
	protected.GET("/products/:id", productHandler.GetProduct)
	protected.PUT("/products/:id", productHandler.UpdateProduct)
	protected.DELETE("/products/:id", productHandler.DeleteProduct)
	protected.GET("/products/:id/snapshots", productHandler.GetProductSnapshots)

	e.Logger.Fatal(e.Start(":8080"))
	return e
}

func (app *Application) Run(h http.Handler) error {
	srv := &http.Server{
		Addr:         app.Config.WebServerPort,
		Handler:      h,
		WriteTimeout: time.Second * 30,
		ReadTimeout:  time.Second * 10,
		IdleTimeout:  time.Minute,
	}

	log.Printf("server has started at addr %s", app.Config.WebServerPort)

	return srv.ListenAndServeTLS("server.crt", "server.key")
}
