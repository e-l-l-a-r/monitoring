package auditor

import (
	"os"
	"sync"

	"github.com/e-l-l-a-r/monitoring/internal/logger"
)

type fileAuditor struct {
	baseObserver
	fileName string
	file     *os.File
	mtx      sync.Mutex
}

// NewFileAuditor создает нового наблюдателя, который записывает данные аудита в файл.
func NewFileAuditor(fileName string) (*fileAuditor, error) {
	file, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, logger.NewTracedError("Error opening file "+fileName+": ", err)
	}

	return &fileAuditor{
		fileName: fileName,
		file:     file,
		baseObserver: baseObserver{
			id: "File:" + fileName,
		},
	}, nil
}

func (f *fileAuditor) Close() error {
	f.mtx.Lock()
	defer f.mtx.Unlock()

	if err := f.file.Close(); err != nil {
		return logger.NewTracedError("Error closing file "+f.fileName+": ", err)
	}
	return nil
}

func (f *fileAuditor) update(data *AuditData) error {
	strData, err := f.baseObserver.prepareData(data)
	if err != nil {
		return logger.NewTracedError("audit data prepare error", err)
	}

	f.mtx.Lock()
	defer f.mtx.Unlock()

	_, err = f.file.WriteString(strData + "\n") // Добавляем перенос строки
	if err != nil {
		return logger.NewTracedError("Error writing to file "+f.fileName+": ", err)
	}

	return nil
}
