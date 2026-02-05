package user

import (
	"context"
	"errors"

	"github.com/freitasmatheusrn/lifecycle-monitor/internal/database"
	repo "github.com/freitasmatheusrn/lifecycle-monitor/internal/database/postgres/sqlc"
	"github.com/freitasmatheusrn/lifecycle-monitor/pkg/rest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"
)

type Service interface {
	FindByID(ctx context.Context, userID pgtype.UUID) (UserOutput, *rest.ApiErr)
	FindByEmail(ctx context.Context, email string) (repo.User, *rest.ApiErr)
	UpdateUser(ctx context.Context, userID pgtype.UUID, input UpdateUserInput) (UserOutput, *rest.ApiErr)
}

type svc struct {
	repo repo.Querier
}

func NewService(repo repo.Querier) Service {
	return &svc{repo: repo}
}

func (s *svc) FindByID(ctx context.Context, userID pgtype.UUID) (UserOutput, *rest.ApiErr) {
	user, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return UserOutput{}, rest.NewNotFoundError("usuário não encontrado")
		}
		if pgErr, ok := err.(*pgconn.PgError); ok {
			return UserOutput{}, database.GetError(pgErr, pgErr.ColumnName)
		}
		return UserOutput{}, rest.NewInternalServerError("erro interno do servidor")
	}

	return toUserOutput(user), nil
}

func (s *svc) FindByEmail(ctx context.Context, email string) (repo.User, *rest.ApiErr) {
	user, err := s.repo.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return repo.User{}, rest.NewNotFoundError("usuário não encontrado")
		}
		if pgErr, ok := err.(*pgconn.PgError); ok {
			return repo.User{}, database.GetError(pgErr, pgErr.ColumnName)
		}
		return repo.User{}, rest.NewInternalServerError("erro interno do servidor")
	}

	return user, nil
}

func (s *svc) UpdateUser(ctx context.Context, userID pgtype.UUID, input UpdateUserInput) (UserOutput, *rest.ApiErr) {
	params := repo.UpdateUserParams{
		ID: userID,
	}

	if input.Name != nil {
		params.Name = pgtype.Text{String: *input.Name, Valid: true}
	}

	if input.Email != nil {
		params.Email = pgtype.Text{String: *input.Email, Valid: true}
	}

	if input.Password != nil {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(*input.Password), bcrypt.DefaultCost)
		if err != nil {
			return UserOutput{}, rest.NewInternalServerError("erro ao processar senha")
		}
		params.Password = pgtype.Text{String: string(hashedPassword), Valid: true}
	}

	user, err := s.repo.UpdateUser(ctx, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return UserOutput{}, rest.NewNotFoundError("usuário não encontrado")
		}
		if pgErr, ok := err.(*pgconn.PgError); ok {
			return UserOutput{}, database.GetError(pgErr, pgErr.ColumnName)
		}
		return UserOutput{}, rest.NewInternalServerError("erro interno do servidor")
	}

	return toUserOutput(user), nil
}

func toUserOutput(user repo.User) UserOutput {
	return UserOutput{
		ID:        user.ID,
		Name:      user.Name,
		Email:     user.Email,
		CreatedAt: user.CreatedAt.Time,
	}
}
