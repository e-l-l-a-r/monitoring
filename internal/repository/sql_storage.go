package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/e-l-l-a-r/monitoring/internal/logger"
	"github.com/e-l-l-a-r/monitoring/internal/model"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/jackc/pgx/v5/stdlib"

	_ "github.com/golang-migrate/migrate/v4/source/file"
)

type SqlStorage struct {
	MemStorage
	db *sql.DB
}

func NewSqlStorage(connectionStr string) (res *SqlStorage, err error) {
	db, err := sql.Open("pgx", connectionStr)
	if err != nil {
		return nil, err
	}

	return &SqlStorage{
		MemStorage: *NewMemStorage(0, "/dev/null"),
		db:         db,
	}, nil
}

func (sql *SqlStorage) Close() {
	sql.db.Close()
}

func (sql *SqlStorage) Ping() error {
	return sql.db.Ping()
}
func (sql *SqlStorage) DoMigrate() error {
	driver, err := postgres.WithInstance(sql.db, &postgres.Config{})
	if err != nil {
		logger.Warn("Error creating migrations driver: ", err)
		return err
	}
	m, err := migrate.NewWithDatabaseInstance(
		"file://./migrations",
		"postgres", driver)
	if err != nil {
		logger.Warn("Error creating migrations: ", err)
		return err
	}
	m.Up()
	return nil
}

func (sqls *SqlStorage) addMeta(сtx context.Context, name string, mtype string) error {
	_, err := sqls.db.ExecContext(сtx, "INSERT INTO metrics_meta(name, mtype) values ($1, $2)",
		name,
		mtype,
	)
	if err != nil {
		logger.Warn("Error adding metric ", name, " metadata to DB: ", err)
		return err
	}
	return nil
}

func (sqls *SqlStorage) addVal(сtx context.Context, name string, mtype string, value interface{}) error {
	table_name := "gauge_values"
	if mtype == model.Counter {
		table_name = "counter_values"
	}

	_, err := sqls.db.ExecContext(сtx, `
		WITH meta as (select *
					  from metrics_meta
					  where name = $1 and mtype = $2)
		INSERT
		INTO `+table_name+` (metric_id, value, datetime)
		SELECT id, $3, now()
		FROM meta`,
		name, mtype, value,
	)

	if err != nil {
		logger.Warn("Error adding value for ", name, " metadata to DB: ", err)
		return err
	}
	return nil
}

func (sqls *SqlStorage) saveMetric(сtx context.Context, name string, is_new bool) error {
	val, ok := sqls.Metrics[name]
	if !ok {
		return fmt.Errorf("no metric for save")
	}
	if is_new {
		err := sqls.addMeta(сtx, name, val.MType)
		if err != nil {
			return err
		}
	}

	switch val.MType {
	case model.Counter:
		return sqls.addVal(сtx, name, val.MType, val.Delta)
	case model.Gauge:
		return sqls.addVal(сtx, name, val.MType, val.Value)
	}

	return nil
}

func (sqls *SqlStorage) AddData(ctx context.Context, name string, mtype string, value interface{}) error {
	_, ok := sqls.Metrics[name]
	err := sqls.MemStorage.AddData(ctx, name, mtype, value)
	if err != nil {
		logger.Warn("Error adding metric ", name, " : ", err)
		return err
	}

	return sqls.saveMetric(ctx, name, !ok)
}

func (sqls *SqlStorage) AddMetricData(ctx context.Context, metric model.Metrics) error {
	val, ok := sqls.Metrics[metric.ID]
	err := sqls.MemStorage.AddMetricData(ctx, metric)
	if err != nil {
		logger.Warn("Error adding metric ", val.ID, " : ", err)
		return err
	}

	return sqls.saveMetric(ctx, metric.ID, !ok)
}

func (sqls *SqlStorage) Restore(ctx context.Context) error {
	for _, mtype := range model.AllTypes {
		tbl := ""
		switch mtype {
		case model.Counter:
			tbl = "counter_values"
		case model.Gauge:
			tbl = "gauge_values"
		}
		rows, err := sqls.db.QueryContext(ctx, `
			SELECT m.name, gv.value 
				FROM metrics_meta m 
				INNER JOIN LATERAL (
				SELECT value FROM `+tbl+`
				WHERE metric_id = m.id ORDER BY datetime DESC LIMIT 1
				) gv ON true
				WHERE m.mtype = $1`,
			mtype,
		)
		if err != nil {
			logger.Warn("Error querying "+mtype+" metrics: ", err)
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var name string
			switch mtype {
			case model.Counter:
				var value int64
				if err := rows.Scan(&name, &value); err != nil {
					logger.Warn("Error scanning counter row: ", err)
					return err
				}
				sqls.MemStorage.Metrics[name] = model.NewCounterMetrics(name, value)
			case model.Gauge:
				var value float64
				if err := rows.Scan(&name, &value); err != nil {
					logger.Warn("Error scanning gauge row: ", err)
					return err
				}
				sqls.MemStorage.Metrics[name] = model.NewGaugeMetrics(name, value)
			}
		}
	}
	return nil
}
