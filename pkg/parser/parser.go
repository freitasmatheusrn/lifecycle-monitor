package parser

import (
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func PgUUIDFromString(id string) (pgtype.UUID, error) {
	u, err := uuid.Parse(id)
	if err != nil {
		return pgtype.UUID{}, err
	}

	var pgUUID pgtype.UUID
	copy(pgUUID.Bytes[:], u[:])
	pgUUID.Valid = true

	return pgUUID, nil
}

func PgUUIDToString(id pgtype.UUID) (string, error) {
	if !id.Valid {
		return "", errors.New("id inválido")
	}

	u, err := uuid.FromBytes(id.Bytes[:])
	if err != nil {
		return "", err
	}

	return u.String(), nil
}