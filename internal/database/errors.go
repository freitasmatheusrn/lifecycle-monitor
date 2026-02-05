package database

import (
	"fmt"
	"strings"

	"github.com/freitasmatheusrn/lifecycle-monitor/pkg/rest"
	"github.com/jackc/pgx/v5/pgconn"
)

var errorMap = map[string]string{
	//UniqueViolation
	"23505": "já está em uso",
	//NotNullViolation
	"23502": "não pode ser nulo",
}

func GetError(err *pgconn.PgError, constraint string) *rest.ApiErr {
	var columnName string
	parts := strings.Split(constraint, "_")
	if len(parts) >= 3 {
		columnName = parts[1]
	}
	if message, ok := errorMap[err.Code]; ok {
		fmtMsg := fmt.Sprintf("%s %s", columnName, message)
		cause := rest.Causes{
			Field:   columnName,
			Message: fmtMsg,
		}
		return rest.NewBadRequestValidationError(fmtMsg, []rest.Causes{cause})
	}
	return rest.NewInternalServerError("erro ao inserir dados")
}
