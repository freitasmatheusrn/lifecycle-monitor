package areas

import (
	"context"
	"errors"

	"github.com/freitasmatheusrn/lifecycle-monitor/internal/database"
	repo "github.com/freitasmatheusrn/lifecycle-monitor/internal/database/postgres/sqlc"
	"github.com/freitasmatheusrn/lifecycle-monitor/pkg/rest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

type Service interface {
	CreateArea(ctx context.Context, input CreateAreaInput) (*AreaOutput, *rest.ApiErr)
	ListAreas(ctx context.Context) ([]AreaOutput, *rest.ApiErr)
	GetArea(ctx context.Context, areaID pgtype.UUID) (*AreaWithCountOutput, *rest.ApiErr)
	UpdateArea(ctx context.Context, areaID pgtype.UUID, input UpdateAreaInput) (*AreaOutput, *rest.ApiErr)
	DeleteArea(ctx context.Context, areaID pgtype.UUID) *rest.ApiErr
}

type svc struct {
	repo repo.Querier
}

func NewService(repo repo.Querier) Service {
	return &svc{repo: repo}
}

func (s *svc) CreateArea(ctx context.Context, input CreateAreaInput) (*AreaOutput, *rest.ApiErr) {
	var description pgtype.Text
	if input.Description != "" {
		description = pgtype.Text{String: input.Description, Valid: true}
	}

	area, err := s.repo.CreateArea(ctx, repo.CreateAreaParams{
		Name:        input.Name,
		Description: description,
	})

	if err != nil {
		return nil, s.handleDBError(err)
	}

	return toAreaOutput(area), nil
}

func (s *svc) ListAreas(ctx context.Context) ([]AreaOutput, *rest.ApiErr) {
	areas, err := s.repo.ListAreas(ctx)
	if err != nil {
		return nil, s.handleDBError(err)
	}

	result := make([]AreaOutput, 0, len(areas))
	for _, a := range areas {
		result = append(result, *toAreaOutput(a))
	}

	return result, nil
}

func (s *svc) GetArea(ctx context.Context, areaID pgtype.UUID) (*AreaWithCountOutput, *rest.ApiErr) {
	area, err := s.repo.FindAreaByID(ctx, areaID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, rest.NewNotFoundError("area nao encontrada")
		}
		return nil, s.handleDBError(err)
	}

	count, err := s.repo.CountProductsByArea(ctx, areaID)
	if err != nil {
		count = 0
	}

	output := toAreaOutput(area)
	return &AreaWithCountOutput{
		AreaOutput:   *output,
		ProductCount: count,
	}, nil
}

func (s *svc) UpdateArea(ctx context.Context, areaID pgtype.UUID, input UpdateAreaInput) (*AreaOutput, *rest.ApiErr) {
	// Check if area exists
	_, err := s.repo.FindAreaByID(ctx, areaID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, rest.NewNotFoundError("area nao encontrada")
		}
		return nil, s.handleDBError(err)
	}

	var name pgtype.Text
	if input.Name != nil {
		name = pgtype.Text{String: *input.Name, Valid: true}
	}

	var description pgtype.Text
	if input.Description != nil {
		description = pgtype.Text{String: *input.Description, Valid: true}
	}

	area, err := s.repo.UpdateArea(ctx, repo.UpdateAreaParams{
		ID:          areaID,
		Name:        name,
		Description: description,
	})

	if err != nil {
		return nil, s.handleDBError(err)
	}

	return toAreaOutput(area), nil
}

func (s *svc) DeleteArea(ctx context.Context, areaID pgtype.UUID) *rest.ApiErr {
	// Check if area exists
	_, err := s.repo.FindAreaByID(ctx, areaID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return rest.NewNotFoundError("area nao encontrada")
		}
		return s.handleDBError(err)
	}

	// Check if area has products
	count, _ := s.repo.CountProductsByArea(ctx, areaID)
	if count > 0 {
		return rest.NewBadRequestError("nao e possivel remover uma area que possui produtos")
	}

	err = s.repo.DeleteArea(ctx, areaID)
	if err != nil {
		return s.handleDBError(err)
	}

	return nil
}

func (s *svc) handleDBError(err error) *rest.ApiErr {
	if errors.Is(err, pgx.ErrNoRows) {
		return rest.NewNotFoundError("recurso nao encontrado")
	}
	if pgErr, ok := err.(*pgconn.PgError); ok {
		return database.GetError(pgErr, pgErr.ConstraintName)
	}
	return rest.NewInternalServerError("erro interno do servidor")
}

func toAreaOutput(a repo.Area) *AreaOutput {
	return &AreaOutput{
		ID:          a.ID,
		Name:        a.Name,
		Description: a.Description.String,
		CreatedAt:   a.CreatedAt.Time,
	}
}
