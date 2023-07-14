# ppdftb
Набор утилит командной строки для работы с pdf файлами:
* wconv - word converter - конвертирует с помощью установленного на компьютере Microsoft Word файлы doc, docx в pdf
* toc 	- table of content - формирует из набора pdf файлов и шаблона в docx содержание с нумерацией страниц в файл docx
* mpdf 	- merge pdf - объединяет набор pdf файлов в один

Примерный сценарий использования:
wconv.exe -id=DOCFOLDER -od=PDFFOLDER //конвертируем все файлы word в pdf
toc.exe -tf="DOCTEMPLATE\3. Содержание.docx" -tn=3 -pd=PDFFOLDER -td=DOCFOLDER //делаем предварительную версию файла содержания на основе шаблона
wconv.exe -if="DOC\3. Содержание.docx" -od=PDFFOLDER //делаем pdf из содержания
toc.exe -tf="DOCTEMPLATE\3. Содержание.docx" -tn=3 -pd=PDFFOLDER -td=DOCFOLDER //делаем финальную версию файла содержания на основе шаблона
wconv.exe -if="DOCFOLDER\3. Содержание.docx" -od=PDFFOLDER //делаем pdf из содержания
mpdf.exe -d=PDFFOLDER -o="All.pdf"  //собираем все pdf в один файл

Благодарности:
* компании Microsoft за замечательный офисный пакет
* FoxyUtils.com за библиотеку unipdf
* автору библиотеки github.com/briiC/docxplate