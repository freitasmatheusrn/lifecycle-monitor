package auth

import (
	"context"
	"errors"
	"net/url"
	"time"

	"github.com/freitasmatheusrn/lifecycle-monitor/internal/database"
	repo "github.com/freitasmatheusrn/lifecycle-monitor/internal/database/postgres/sqlc"
	"github.com/freitasmatheusrn/lifecycle-monitor/pkg/auth"
	"github.com/freitasmatheusrn/lifecycle-monitor/pkg/parser"
	"github.com/freitasmatheusrn/lifecycle-monitor/pkg/rest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"
)

type Service interface {
	Signup(ctx context.Context, input SignupInput, userAgent, ip string) (*SignupResult, *rest.ApiErr)
	Signin(ctx context.Context, credentials SigninInput, userAgent, ip string) (*SigninResult, *rest.ApiErr)
	RefreshTokens(ctx context.Context, refreshToken, userAgent, ip string) (*TokenPair, *rest.ApiErr)
	Logout(ctx context.Context, refreshToken string) error
	LogoutAll(ctx context.Context, userID string) error
}

type service struct {
	repo            repo.Querier
	tokenRepo       TokenRepository
	jwtSecret       string
	accessTokenExp  int
	refreshTokenExp int
}

func NewService(querier repo.Querier, tokenRepo TokenRepository, jwtSecret string, accessExp, refreshExp int) Service {
	return &service{
		repo:            querier,
		tokenRepo:       tokenRepo,
		jwtSecret:       jwtSecret,
		accessTokenExp:  accessExp,
		refreshTokenExp: refreshExp,
	}
}

func (s *service) Signup(ctx context.Context, input SignupInput, userAgent, ip string) (*SignupResult, *rest.ApiErr) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, rest.NewInternalServerError("erro ao processar senha")
	}

	params := repo.CreateUserParams{
		Name:     input.Name,
		Email:    input.Email,
		Password: string(hashedPassword),
	}

	user, err := s.repo.CreateUser(ctx, params)
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok {
			return nil, database.GetError(pgErr, pgErr.ConstraintName)
		}
		return nil, rest.NewInternalServerError("erro ao criar usuário")
	}

	userID, _ := parser.PgUUIDToString(user.ID)
	tokens, err := s.generateTokenPair(ctx, userID, user.Email, userAgent, ip)
	if err != nil {
		return nil, rest.NewInternalServerError("erro ao gerar tokens")
	}

	return &SignupResult{
		User: SignupOutput{
			ID:    user.ID,
			Name:  user.Name,
			Email: user.Email,
		},
		Tokens: tokens,
	}, nil
}

func (s *service) Signin(ctx context.Context, credentials SigninInput, userAgent, ip string) (*SigninResult, *rest.ApiErr) {
	user, err := s.repo.FindByEmail(ctx, credentials.Email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, rest.NewUnauthorizedRequestError("email não encontrado")
		}

		return nil, rest.NewInternalServerError("erro interno do servidor")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(credentials.Password)); err != nil {
		return nil, rest.NewUnauthorizedRequestError("credenciais inválidas")
	}

	userID, _ := parser.PgUUIDToString(user.ID)
	tokens, err := s.generateTokenPair(ctx, userID, user.Email, userAgent, ip)
	if err != nil {
		return nil, rest.NewInternalServerError("erro ao gerar tokens")
	}

	return &SigninResult{
		User: SigninOutput{
			ID:    user.ID,
			Name:  user.Name,
			Email: user.Email,
		},
		Tokens: tokens,
	}, nil
}

func (s *service) RefreshTokens(ctx context.Context, refreshToken, userAgent, ip string) (*TokenPair, *rest.ApiErr) {
	decodedToken, err := url.QueryUnescape(refreshToken)
	if err != nil {
		return nil, rest.NewUnauthorizedRequestError("refresh token inválido")
	}
	tokenHash := auth.HashToken(decodedToken)

	tokenData, err := s.tokenRepo.GetToken(ctx, tokenHash)
	if err != nil {
		return nil, rest.NewInternalServerError("erro ao validar token")
	}
	if tokenData == nil {
		return nil, rest.NewUnauthorizedRequestError("refresh token inválido ou expirado")
	}

	userID, err := parseUUID(tokenData.UserID)
	if err != nil {
		return nil, rest.NewInternalServerError("erro ao processar ID do usuário")
	}

	user, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return nil, rest.NewUnauthorizedRequestError("usuário não encontrado")
	}

	if err := s.tokenRepo.RevokeToken(ctx, tokenHash); err != nil {
		return nil, rest.NewInternalServerError("erro ao revogar token antigo")
	}

	newRefreshToken, err := auth.GenerateRefreshToken()
	if err != nil {
		return nil, rest.NewInternalServerError("erro ao gerar novo token")
	}

	newTokenHash := auth.HashToken(newRefreshToken)
	ttl := time.Duration(s.refreshTokenExp) * time.Second

	if err := s.tokenRepo.StoreToken(ctx, newTokenHash, tokenData.UserID, tokenData.FamilyID, userAgent, ip, ttl); err != nil {
		return nil, rest.NewInternalServerError("erro ao armazenar novo token")
	}

	jwtClaims := auth.NewClaims(tokenData.UserID, user.Email, s.accessTokenExp)
	accessToken, err := auth.GenerateJWT(jwtClaims, s.jwtSecret)
	if err != nil {
		return nil, rest.NewInternalServerError("erro ao gerar access token")
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
	}, nil
}

func (s *service) Logout(ctx context.Context, refreshToken string) error {
	tokenHash := auth.HashToken(refreshToken)
	return s.tokenRepo.RevokeToken(ctx, tokenHash)
}

func (s *service) LogoutAll(ctx context.Context, userID string) error {
	return s.tokenRepo.RevokeAllUserTokens(ctx, userID)
}

func (s *service) generateTokenPair(ctx context.Context, userID, email, userAgent, ip string) (*TokenPair, error) {
	jwtClaims := auth.NewClaims(userID, email, s.accessTokenExp)
	accessToken, err := auth.GenerateJWT(jwtClaims, s.jwtSecret)
	if err != nil {
		return nil, err
	}

	refreshToken, err := auth.GenerateRefreshToken()
	if err != nil {
		return nil, err
	}

	tokenHash := auth.HashToken(refreshToken)
	familyID := auth.GenerateFamilyID()
	ttl := time.Duration(s.refreshTokenExp) * time.Second

	if err := s.tokenRepo.StoreToken(ctx, tokenHash, userID, familyID, userAgent, ip, ttl); err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

func parseUUID(id string) (pgtype.UUID, error) {
	var pgUUID pgtype.UUID
	err := pgUUID.Scan(id)
	return pgUUID, err
}
