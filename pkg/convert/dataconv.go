package convert

import "github.com/Nemo08/ppdftb/pkg/dataconv"

// DataMerge объединяет несколько XML-документов в один JSON.
// Первый документ — база; каждый следующий перезаписывает/добавляет поля.
// Используется для слияния XML-данных из нескольких источников перед
// подстановкой в шаблон DOCX.
func DataMerge(data [][]byte) ([]byte, error) {
	return dataconv.DataMerge(data)
}
