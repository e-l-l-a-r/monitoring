package logger

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"runtime"
	"time"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
)

type (
	TracedError struct {
		time       time.Time
		err        error
		tracePoint string
		msg        string
	}
	ErrorClassification int
)

var retryDelays = []time.Duration{
	time.Second,
	time.Second * 3,
	time.Second * 5,
}

const (
	// NonRetriable - операцию не следует повторять
	NonRetriable ErrorClassification = iota

	// Retriable - операцию можно повторить
	Retriable
)

func getCallerInfo(skip int) (file string, line int) {
	_, file, line, ok := runtime.Caller(skip)
	if !ok {
		return "unknown", 0
	}
	return file, line
}

func NewTracedError(msg string, err error) *TracedError {
	file, line := getCallerInfo(1) // пропускаем текущую функцию
	singleLogger.Sugar().Warnln(msg, "error", err.Error())
	return &TracedError{
		time:       time.Now(),
		err:        err,
		tracePoint: fmt.Sprintf("%s:%d", filepath.Base(file), line),
		msg:        msg,
	}
}

func (te *TracedError) Error() string {
	return te.msg
}

func (te *TracedError) Unwrap() error {
	return te.err
}

func (te *TracedError) IsRetriable() bool {
	if te.err == nil {
		return false
	}

	// Проверяем и конвертируем в pgconn.PgError, если это возможно
	var pgErr *pgconn.PgError
	if errors.As(te.err, &pgErr) {
		return СlassifyPgError(pgErr) == Retriable
	}

	var urlErr *url.Error
	if errors.As(te.err, &urlErr) {
		return true
	}

	// По умолчанию считаем ошибку неповторяемой
	return false
}

func ExecuteWithRetry(f func(args ...interface{}) (interface{}, error), args ...interface{}) (interface{}, error) {
	var err error
	var val interface{}

	for i := 0; i < len(retryDelays); i++ {
		val, err = f(args)

		if err == nil {
			return val, nil
		}
		te := NewTracedError("Error while executing: ", err)
		if !te.IsRetriable() {
			return nil, err
		}
		singleLogger.Sugar().Warnln("Retrying after " + retryDelays[i].String())
		time.Sleep(retryDelays[i])
	}
	return nil, err
}

func ExecuteWithRetryNoResult(f func(args ...interface{}) error, args ...interface{}) error {
	_, err := ExecuteWithRetry(func(args ...interface{}) (interface{}, error) {
		return nil, f(args...)
	}, args...)
	return err
}

func СlassifyPgError(pgErr *pgconn.PgError) ErrorClassification {
	// Коды ошибок PostgreSQL: https://www.postgresql.org/docs/current/errcodes-appendix.html

	switch pgErr.Code {
	// Класс 08 - Ошибки соединения
	case pgerrcode.ConnectionException,
		pgerrcode.ConnectionDoesNotExist,
		pgerrcode.ConnectionFailure:
		return Retriable

	// Класс 40 - Откат транзакции
	case pgerrcode.TransactionRollback, // 40000
		pgerrcode.SerializationFailure, // 40001
		pgerrcode.DeadlockDetected:     // 40P01
		return Retriable

	// Класс 57 - Ошибка оператора
	case pgerrcode.CannotConnectNow: // 57P03
		return Retriable
	}

	// Можно добавить более конкретные проверки с использованием констант pgerrcode
	switch pgErr.Code {
	// Класс 22 - Ошибки данных
	case pgerrcode.DataException,
		pgerrcode.NullValueNotAllowedDataException:
		return NonRetriable

	// Класс 23 - Нарушение ограничений целостности
	case pgerrcode.IntegrityConstraintViolation,
		pgerrcode.RestrictViolation,
		pgerrcode.NotNullViolation,
		pgerrcode.ForeignKeyViolation,
		pgerrcode.UniqueViolation,
		pgerrcode.CheckViolation:
		return NonRetriable

	// Класс 42 - Синтаксические ошибки
	case pgerrcode.SyntaxErrorOrAccessRuleViolation,
		pgerrcode.SyntaxError,
		pgerrcode.UndefinedColumn,
		pgerrcode.UndefinedTable,
		pgerrcode.UndefinedFunction:
		return NonRetriable
	}

	// По умолчанию считаем ошибку неповторяемой
	return NonRetriable
}
