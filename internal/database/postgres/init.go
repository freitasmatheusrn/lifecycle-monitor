package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func Init(conn string) (*pgx.Conn, error) {
	ctx := context.Background()
	db, err := pgx.Connect(ctx, conn)
	if err != nil {
		return nil, err
	}
	err = db.Ping(ctx)
	if err != nil {
		return nil, err
	}
	return db, nil
}
