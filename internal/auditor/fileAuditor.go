package auditor

import (
	"os"

	"github.com/e-l-l-a-r/monitoring/internal/logger"
)

type fileAuditor struct {
	baseObserver
	file string
}

func NewFileAuditor(file string) *fileAuditor {
	return &fileAuditor{
		file: file,
		baseObserver: baseObserver{
			id: "File:" + file,
		},
	}
}

func (f *fileAuditor) update(data *AuditData) error {
	f.baseObserver.prepareData(data)
	out, err := os.OpenFile(f.file, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return logger.NewTracedError("Error opening file "+f.file+": ", err)
	}
	defer out.Close()
	_, err = out.WriteString(f.strData + "\n") // Добавляем перенос строки
	if err != nil {
		return logger.NewTracedError("Error writing to file "+f.file+": ", err)
	}

	return nil
}
