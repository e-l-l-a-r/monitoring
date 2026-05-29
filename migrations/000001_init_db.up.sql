-- Миграция: создание таблицы метаданных метрик
CREATE TABLE IF NOT EXISTS metrics_meta (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    mtype VARCHAR(10) NOT NULL CHECK (mtype IN ('counter', 'gauge'))
    );

-- Индекс для быстрого поиска по имени метрики
CREATE INDEX IF NOT EXISTS idx_metrics_meta_name ON metrics_meta(name);

-- Миграция: создание таблицы значений counter
CREATE TABLE IF NOT EXISTS counter_values (
    id SERIAL PRIMARY KEY,
    metric_id INTEGER NOT NULL REFERENCES metrics_meta(id) ON DELETE CASCADE,
    value BIGINT NOT NULL,
    datetime TIMESTAMP WITH TIME ZONE DEFAULT NOW()
    );

-- Индекс для быстрого поиска значений по метрике и дате
CREATE INDEX IF NOT EXISTS idx_counter_values_metric_id ON counter_values(metric_id,  datetime);

-- Миграция: создание таблицы значений gauge
CREATE TABLE IF NOT EXISTS gauge_values (
    id SERIAL PRIMARY KEY,
    metric_id INTEGER NOT NULL REFERENCES metrics_meta(id) ON DELETE CASCADE,
    value DOUBLE PRECISION NOT NULL,
    datetime TIMESTAMP WITH TIME ZONE DEFAULT NOW()
    );

-- Индекс для быстрого поиска значений по метрике и дате
CREATE INDEX IF NOT EXISTS idx_gauge_values_metric_id ON gauge_values(metric_id, datetime);
