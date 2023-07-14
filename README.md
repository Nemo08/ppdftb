# ppdftb
### Набор утилит командной строки для работы с pdf файлами:
* wconv - word converter - конвертирует с помощью установленного на компьютере Microsoft Word файлы doc, docx в pdf
* toc 	- table of content - формирует из набора pdf файлов и шаблона в docx содержание с нумерацией страниц в файл docx
* mpdf 	- merge pdf - объединяет набор pdf файлов в один

### Примерный сценарий использования:
* wconv.exe -id=DOCFOLDER -od=PDFFOLDER //конвертируем все файлы word в pdf
* toc.exe -tf="DOCTEMPLATE\3. Содержание.docx" -tn=3 -pd=PDFFOLDER -td=DOCFOLDER //делаем предварительную версию файла содержания на основе шаблона
* wconv.exe -if="DOCFOLDER\3. Содержание.docx" -od=PDFFOLDER //делаем pdf из содержания
* toc.exe -tf="DOCTEMPLATE\3. Содержание.docx" -tn=3 -pd=PDFFOLDER -td=DOCFOLDER //делаем финальную версию файла содержания на основе шаблона
* wconv.exe -if="DOCFOLDER\3. Содержание.docx" -od=PDFFOLDER //делаем pdf из содержания
* mpdf.exe -d=PDFFOLDER -o="All.pdf"  //собираем все pdf в один файл

### Благодарности:
* компании Microsoft за замечательный офисный пакет
* FoxyUtils.com за библиотеку unipdf
* автору библиотеки github.com/briiC/docxplate

### A set of useful command lines for working with pdf files:
* wconv - word converter - converts doc, docx files to pdf using Microsoft Word installed on your computer
* toc - table of content - generates from a set of pdf files and a template in docx the content with pagination in the docx file
* mpdf - merge pdf - merge pdf files into one

An example of using the script:
* wconv.exe -id=DOCFOLDER -od=PDFFOLDER //convert all word files to pdf
* toc.exe -tf="DOCTEMPLATE\3.Content.docx" -tn=3 -pd=PDFFOLDER -td=DOCFOLDER //make a preview file based on the template
* wconv.exe -if="DOCFOLDER\3.Content.docx" -od=PDFFOLDER //make pdf from content
* toc.exe -tf="DOCTEMPLATE\3.Content.docx" -tn=3 -pd=PDFFOLDER -td=DOCFOLDER //make the final version of the content file based on the template
* wconv.exe -if="DOCFOLDER\3.Content.docx" -od=PDFFOLDER //make pdf from content
* mpdf.exe -d=PDFFOLDER -o="All.pdf" // collect all pdf into one file
### Thanks:
* Microsoft for a wonderful office suite
* FoxyUtils.com to download unipdf
* to the author of the library github.com/briiC/docxplate
