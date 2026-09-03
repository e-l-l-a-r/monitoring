package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/e-l-l-a-r/monitoring/internal/logger"
	"github.com/e-l-l-a-r/monitoring/internal/model"

	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// SQLStorage предоставляет реализацию хранилища метрик в базе данных SQL.
// Расширяет MemStorage, обеспечивая персистентность.
type SQLStorage struct {
	MemStorage         // Внутреннее хранилище в памяти для быстрого доступа
	db         *sql.DB // Подключение к базе данных
}

type metadata struct {
	name  string
	mtype string
}

// NewSQLStorage создает новый экземпляр SQLStorage с подключением к БД.
func NewSQLStorage(connectionStr string) (res *SQLStorage, err error) {
	db, err := sql.Open("pgx", connectionStr)
	if err != nil {
		return nil, err
	}

	return &SQLStorage{
		MemStorage: *NewMemStorage(0, "/dev/null"),
		db:         db,
	}, nil
}

// Close закрывает соединение с базой данных.
func (storage *SQLStorage) Close() {
	_ = storage.db.Close()
}

// Ping проверяет доступность базы данных.
func (storage *SQLStorage) Ping() error {
	return storage.db.Ping()
}

// DoMigrate выполняет миграции базы данных.
func (storage *SQLStorage) DoMigrate() error {
	driver, err := postgres.WithInstance(storage.db, &postgres.Config{})
	if err != nil {
		return logger.NewTracedError("Error creating migrations driver: ", err)
	}
	m, err := migrate.NewWithDatabaseInstance(
		"file://./migrations",
		"postgres", driver)
	if err != nil {
		return logger.NewTracedError("Error creating migrations: ", err)
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return logger.NewTracedError("Error applying migrations: ", err)
	}
	return nil
}

func (storage *SQLStorage) addMeta(ctx context.Context, name string, mtype string) error {
	_, err := storage.db.ExecContext(ctx, "INSERT INTO metrics_meta(name, mtype) values ($1, $2)",
		name,
		mtype,
	)
	if err != nil {
		return logger.NewTracedError("Error adding metric "+name+" metadata to DB: ", err)
	}
	return nil
}

func (storage *SQLStorage) addVal(ctx context.Context, name string, mtype string, value interface{}) error {
	tableName := "gauge_values"
	if mtype == model.Counter {
		tableName = "counter_values"
	}

	_, err := storage.db.ExecContext(ctx, `
		WITH meta as (select *
					  from metrics_meta
					  where name = $1 and mtype = $2)
		INSERT
		INTO `+tableName+` (metric_id, value, datetime)
		SELECT id, $3, now()
		FROM meta`,
		name, mtype, value,
	)

	if err != nil {
		return logger.NewTracedError("Error adding value for "+name+" metadata to DB: ", err)
	}
	return nil
}

func (storage *SQLStorage) saveMetric(ctx context.Context, name string, isNew bool) error {
	val, ok := storage.Metrics[name]
	if !ok {
		return fmt.Errorf("no metric for save")
	}
	if isNew {
		err := storage.addMeta(ctx, name, val.MType)
		if err != nil {
			return err
		}
	}

	switch val.MType {
	case model.Counter:
		return storage.addVal(ctx, name, val.MType, val.Delta)
	case model.Gauge:
		return storage.addVal(ctx, name, val.MType, val.Value)
	}

	return nil
}

func (storage *SQLStorage) AddData(ctx context.Context, name string, mtype string, value interface{}) error {
	_, ok := storage.Metrics[name]
	err := storage.MemStorage.AddData(ctx, name, mtype, value)
	if err != nil {
		return logger.NewTracedError("Error adding metric "+name+" : ", err)
	}

	return storage.saveMetric(ctx, name, !ok)
}

func (storage *SQLStorage) AddMetricData(ctx context.Context, metric model.Metrics) error {
	val, ok := storage.Metrics[metric.ID]
	err := storage.MemStorage.AddMetricData(ctx, metric)
	if err != nil {
		return logger.NewTracedError("Error adding metric "+val.ID+" : ", err)
	}

	return storage.saveMetric(ctx, metric.ID, !ok)
}

func massAddMetadata(ctx context.Context, tx *sql.Tx, metadata []metadata) error {
	stmt, err := tx.PrepareContext(ctx, "INSERT INTO metrics_meta(name, mtype) values ($1, $2) on conflict do nothing ")
	if err != nil {
		return logger.NewTracedError("Error preparing statement: ", err)
	}
	defer func() {
		_ = stmt.Close()
	}()
	for _, m := range metadata {
		_, err := stmt.ExecContext(ctx, m.name, m.mtype)
		if err != nil {
			return logger.NewTracedError("Error adding metric "+m.name+" metadata to DB: ", err)
		}
	}
	return nil
}

func (storage *SQLStorage) massAddValues(ctx context.Context, tx *sql.Tx, values []model.Metrics) error {
	stmtGauge, err := tx.PrepareContext(ctx, `
		WITH meta as (select *
					  from metrics_meta
					  where name = $1 and mtype = $2)
		INSERT
		INTO gauge_values (metric_id, value, datetime)
		SELECT id, $3, now()
		FROM meta`)
	if err != nil {
		return logger.NewTracedError("Error preparing statement for gauge values: ", err)
	}
	defer func() {
		_ = stmtGauge.Close()
	}()
	stmtCounter, err := tx.PrepareContext(ctx, `
		WITH meta as (select *
					  from metrics_meta
					  where name = $1 and mtype = $2)
		INSERT
		INTO counter_values (metric_id, value, datetime)
		SELECT id, $3, now()
		FROM meta`)
	if err != nil {
		return logger.NewTracedError("Error preparing statement for counter values: ", err)
	}
	defer func() {
		_ = stmtCounter.Close()
	}()
	for _, metric := range values {
		err := storage.MemStorage.AddMetricData(ctx, metric)
		if err != nil {
			return logger.NewTracedError("Error adding metric "+metric.ID+" : ", err)
		}
		switch metric.MType {
		case model.Counter:
			_, err = stmtCounter.ExecContext(ctx, metric.ID, metric.MType, metric.Delta)
		case model.Gauge:
			_, err = stmtGauge.ExecContext(ctx, metric.ID, metric.MType, metric.Value)
		}
		if err != nil {
			return logger.NewTracedError("Error adding value for "+metric.ID+" metadata to DB: ", err)
		}
	}
	return nil
}

func (storage *SQLStorage) AddBatchMetricsData(ctx context.Context, metrics []model.Metrics) error {
	newMetadata := make([]metadata, 0)
	for _, metric := range metrics {
		_, ok := storage.Metrics[metric.ID]
		if !ok {
			newMetadata = append(newMetadata, metadata{name: metric.ID, mtype: metric.MType})
			logger.Info("Add new metadata: ", metric.ID)
		}
	}
	tx, err := storage.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return logger.NewTracedError("Error starting transaction: ", err)
	}
	err = massAddMetadata(ctx, tx, newMetadata)
	if err != nil {
		_ = tx.Rollback()
		return logger.NewTracedError("Error adding metadata: ", err)
	}
	err = storage.massAddValues(ctx, tx, metrics)
	if err != nil {
		_ = tx.Rollback()
		return logger.NewTracedError("Error adding values: ", err)
	}
	if err := tx.Commit(); err != nil {
		return logger.NewTracedError("Error committing transaction: ", err)
	}
	return nil
}

// Restore загружает состояние метрик из базы данных в память.
func (storage *SQLStorage) Restore(ctx context.Context) error {
	for _, mtype := range model.AllTypes {
		tbl := ""
		switch mtype {
		case model.Counter:
			tbl = "counter_values"
		case model.Gauge:
			tbl = "gauge_values"
		}
		query := fmt.Sprintf(`
			SELECT m.name, gv.value 
				FROM metrics_meta m 
				INNER JOIN LATERAL (
				SELECT value FROM %s
				WHERE metric_id = m.id ORDER BY datetime DESC LIMIT 1
				) gv ON true
				WHERE m.mtype = $1`, tbl)
		rows, err := storage.db.QueryContext(ctx, query,
			mtype,
		)
		if err != nil {
			return logger.NewTracedError("Error querying "+mtype+" metrics: ", err)
		}
		defer func() {
			_ = rows.Close()
		}()
		for rows.Next() {
			var name string
			switch mtype {
			case model.Counter:
				var value int64
				if err := rows.Scan(&name, &value); err != nil {
					return logger.NewTracedError("Error scanning counter row: ", err)
				}
				storage.MemStorage.Metrics[name] = model.NewCounterMetrics(name, value)
			case model.Gauge:
				var value float64
				if err := rows.Scan(&name, &value); err != nil {
					return logger.NewTracedError("Error scanning gauge row: ", err)
				}
				storage.MemStorage.Metrics[name] = model.NewGaugeMetrics(name, value)
			}
		}
		if err := rows.Err(); err != nil {
			return logger.NewTracedError("Error reading "+mtype+" rows: ", err)
		}
	}
	return nil
}
