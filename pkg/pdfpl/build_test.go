package pdfpl

import (
	"strconv"
	"strings"
	"testing"
)

// helpRun строит ран с логическим текстом (как его даёт textRuns).
func helpRun(text string, pos int) streamRun {
	return streamRun{
		text:  text,
		start: pos,
		end:   pos + len(text),
		font:  "/F1",
		size:  12,
	}
}

func TestSegOfRun(t *testing.T) {
	r := helpRun("xx{{.A}}yy{{.B}}z", 10)
	fields := []field{
		{key: "A", run: 0},
		{key: "B", run: 0},
	}
	segs := segOfRun(r, fields)

	wantText := []string{"xx", "{{.A}}", "yy", "{{.B}}", "z"}
	if len(segs) != 5 {
		t.Fatalf("len(segs) = %d, want 5", len(segs))
	}
	for i, s := range segs {
		if s.text != wantText[i] {
			t.Errorf("segs[%d].text = %q, want %q", i, s.text, wantText[i])
		}
	}
	if segs[1].kind != segValue || segs[1].value != "A" {
		t.Errorf("segs[1]: kind=%v value=%q, want segValue A", segs[1].kind, segs[1].value)
	}
	if segs[3].kind != segValue || segs[3].value != "B" {
		t.Errorf("segs[3]: kind=%v value=%q, want segValue B", segs[3].kind, segs[3].value)
	}
	if segs[0].kind != segText || segs[2].kind != segText || segs[4].kind != segText {
		t.Error("нечётные сегменты должны быть segText")
	}
	// begin/end относительны к началу рана (текст "xx{{.A}}...").
	if segs[1].begin != 2 || segs[1].end != 8 {
		t.Errorf("segs[1].begin/end = %d/%d, want 2/8", segs[1].begin, segs[1].end)
	}
	if segs[3].begin != 10 || segs[3].end != 16 {
		t.Errorf("segs[3].begin/end = %d/%d, want 10/16", segs[3].begin, segs[3].end)
	}
}

func TestSegOfRunSkipsUnknownPlaceholders(t *testing.T) {
	// Поле C не в списке полей рана — плейсхолдер остаётся как обычный текст.
	r := helpRun("{{.A}}{{.C}}tail", 0)
	fields := []field{{key: "A", run: 0}}
	segs := segOfRun(r, fields)

	// Все сегменты в сумме покрывают исходный текст без потерь.
	var sb strings.Builder
	for _, s := range segs {
		sb.WriteString(s.text)
	}
	if sb.String() != "{{.A}}{{.C}}tail" {
		t.Errorf("сумма текста сегментов = %q, want %q", sb.String(), "{{.A}}{{.C}}tail")
	}
	if len(segs) != 3 {
		t.Fatalf("len(segs) = %d, want 3", len(segs))
	}
	if segs[0].kind != segValue || segs[0].text != "{{.A}}" || segs[0].value != "A" {
		t.Errorf("segs[0] = %v %q %q, want segValue {{.A}} A", segs[0].kind, segs[0].text, segs[0].value)
	}
	if segs[1].kind != segText || segs[1].text != "{{.C}}" {
		t.Errorf("segs[1] = %v %q, want segText {{.C}}", segs[1].kind, segs[1].text)
	}
	if segs[2].kind != segText || segs[2].text != "tail" {
		t.Errorf("segs[2] = %v %q, want segText tail", segs[2].kind, segs[2].text)
	}
}

func TestRebuildOperandNoCompensation(t *testing.T) {
	// Полный плейсхолдер {{aa}} = 3000, значение "aaaaaa" = 3000 —
	// точное совпадение, строковая форма Tj без кернинга.
	r := helpRun("{{aa}}", 0)
	fields := []field{{key: "aa", run: 0}}
	values := map[string]string{"aa": "aaaaaa"}

	out, _, _, err := rebuildOperand(r, fields, values, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out != "(aaaaaa) Tj" {
		t.Errorf("rebuildOperand = %q, want %q", out, "(aaaaaa) Tj")
	}
}

func TestRebuildOperandCompensates(t *testing.T) {
	// Значение уже плейсхолдера — TJ с кернингом смещения.
	r := helpRun("T{{.N}}T", 0)
	fields := []field{{key: "N", run: 0}}
	values := map[string]string{"N": "1"}

	out, _, _, err := rebuildOperand(r, fields, values, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	delta := textWidth("{{.N}}") - textWidth("1") // 2928 - 278 = 2650
	want := "[(" + quoteString("T1T") + ") " + strconv.FormatFloat(-delta, 'g', -1, 64) + "] TJ"
	if out != want {
		t.Errorf("rebuildOperand = %q, want %q", out, want)
	}
}

func TestRebuildOperandWiderValueSkips(t *testing.T) {
	r := helpRun("T{{.N}}T", 0)
	fields := []field{{key: "N", run: 0}}
	// 6 широких букв (6*650=3900) шире всего плейсхолдера {{.N}} (2928) — замена отменяется.
	values := map[string]string{"N": "ABCDEF"}

	out, _, _, err := rebuildOperand(r, fields, values, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Errorf("rebuildOperand = %q, want %q (пусто = не трогать)", out, "")
	}
}

func TestRebuildOperandAllowWider(t *testing.T) {
	r := helpRun("T{{.N}}T", 0)
	fields := []field{{key: "N", run: 0}}
	// Значение шире плейсхолдера, но allowWider=true — заменяем без компенсации
	// ширины (дельта отрицательна: текст уходит вправо за пределы исходной линии).
	values := map[string]string{"N": "ABCDEF"}

	out, _, _, err := rebuildOperand(r, fields, values, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "(" + quoteString("TABCDEFT") + ") Tj"
	if out != want {
		t.Errorf("rebuildOperand = %q, want %q", out, want)
	}
}

func TestRebuildOperandMissingKeySkips(t *testing.T) {
	r := helpRun("{{.A}}", 0)
	fields := []field{{key: "A", run: 0}}

	out, _, _, err := rebuildOperand(r, fields, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Errorf("rebuildOperand = %q, want %q (пусто = не трогать)", out, "")
	}
}

func TestRebuildOperandEscapes(t *testing.T) {
	r := helpRun("{{.A}}", 0)
	fields := []field{{key: "A", run: 0}}
	// "(" + "1" — ширина 500+278=778 <= 2928 ({{.A}}), замена допустима.
	values := map[string]string{"A": "(1"}

	out, _, _, err := rebuildOperand(r, fields, values, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	delta := textWidth("{{.A}}") - textWidth("(1")
	want := "[(" + quoteString("(1") + ") -" + strconv.FormatFloat(delta, 'g', -1, 64) + "] TJ"
	if out != want {
		t.Errorf("rebuildOperand = %q, want %q", out, want)
	}
}

func TestBuildStreamNoCompensation(t *testing.T) {
	// Сквозной конвейер: textRuns → findFields → buildStream.
	// {{aa}} (3000) → "aaaaaa" (3000): точное совпадение, Tj без кернинга.
	stream := "BT /F1 12 Tf ({{aa}}) Tj ET"
	runs, err := textRuns(stream)
	if err != nil {
		t.Fatal(err)
	}
	fields := findFields(stream, runs)
	out, err := buildStream(stream, runs, fields, map[string]string{"aa": "aaaaaa"})
	if err != nil {
		t.Fatal(err)
	}
	if out != "BT /F1 12 Tf (aaaaaa) Tj ET" {
		t.Errorf("buildStream = %q, want %q", out, "BT /F1 12 Tf (aaaaaa) Tj ET")
	}
}

func TestBuildStreamCompensation(t *testing.T) {
	stream := "BT /F1 12 Tf (T{{.N}}T) Tj ET"
	runs, err := textRuns(stream)
	if err != nil {
		t.Fatal(err)
	}
	fields := findFields(stream, runs)
	out, err := buildStream(stream, runs, fields, map[string]string{"N": "1"})
	if err != nil {
		t.Fatal(err)
	}
	want := "BT /F1 12 Tf [(T1T) -2650] TJ ET"
	if out != want {
		t.Errorf("buildStream = %q, want %q", out, want)
	}
}

func TestBuildStreamMultipleFields(t *testing.T) {
	stream := "BT /F1 12 Tf ({{.A}}{{.B}}) Tj ET"
	runs, _ := textRuns(stream)
	fields := findFields(stream, runs)
	out, err := buildStream(stream, runs, fields, map[string]string{"A": "1", "B": "7"})
	if err != nil {
		t.Fatal(err)
	}
	// delta = (2928-278) + (2928-500) = 5078
	if !strings.Contains(out, "[(17) -5078] TJ") {
		t.Errorf("buildStream = %q, want содержащий [(17) -5078] TJ", out)
	}
}

func TestBuildStreamTJInput(t *testing.T) {
	// Исходный TJ-массив: пересобирается целиком как TJ.
	stream := "BT /F1 12 Tf [({{.N}})-120(x)] TJ ET"
	runs, err := textRuns(stream)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].text != "{{.N}}x" {
		t.Fatalf("runs = %+v, want 1 run text {{.N}}x", runs)
	}
	fields := findFields(stream, runs)
	out, err := buildStream(stream, runs, fields, map[string]string{"N": "1"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "[(1x) -2650] TJ") {
		t.Errorf("buildStream = %q, want содержащий [(1x) -2650] TJ", out)
	}
}

func TestBuildStreamWiderValueKeepsOriginal(t *testing.T) {
	stream := "BT /F1 12 Tf ({{.N}}) Tj ET"
	runs, _ := textRuns(stream)
	fields := findFields(stream, runs)
	out, err := buildStream(stream, runs, fields, map[string]string{"N": "ZZZZZZ"})
	if err != nil {
		t.Fatal(err)
	}
	if out != stream {
		t.Errorf("buildStream = %q, want без изменений %q", out, stream)
	}
}

func TestQuoteString(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"abc", "abc"},
		{`(\)`, `\(\\\)`},
		{`a(b)\c`, `a\(b\)\\c`},
	}
	for _, c := range cases {
		if got := quoteString(c.in); got != c.want {
			t.Errorf("quoteString(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestGlyphWidth(t *testing.T) {
	cases := []struct {
		r    rune
		want float64
	}{
		{'1', 278}, // узкая цифра
		{'7', 500}, // обычная цифра
		{'I', 278}, // узкая заглавная
		{'A', 650}, // широкая заглавная
		{'a', 500}, // строчная
		{'.', 278}, // узкий знак
		{'{', 500}, // скобка плейсхолдера
		{' ', 278}, // пробел
		{'Ж', 500}, // неизвестный символ — по умолчанию
	}
	for _, c := range cases {
		if got := glyphWidth(c.r); got != c.want {
			t.Errorf("glyphWidth(%q) = %v, want %v", c.r, got, c.want)
		}
	}
}

// Word-стиль: плейсхолдер {{.Key}} разбит на 3 ран-блока ({{.) (Key) (}}).
const crossRunStream = `BT /F1 12 Tf ({{.) Tj ET BT /F1 12 Tf (Key) Tj ET BT /F1 12 Tf (}}) Tj ET`

func TestBuildCrossRunFields(t *testing.T) {
	runs, err := textRuns(crossRunStream)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 3 {
		t.Fatalf("len(runs) = %d, want 3", len(runs))
	}

	vals := map[string]string{"Key": "Bob"}
	cross := buildCrossRunFields(runs, vals)

	if len(cross) != 1 {
		t.Fatalf("len(cross) = %d, want 1", len(cross))
	}
	cf := cross["Key"]
	if cf == nil {
		t.Fatal("field Key не найден")
	}
	if len(cf.parts) != 3 {
		t.Fatalf("len(parts) = %d, want 3", len(cf.parts))
	}
	// Фрагменты по рaнам.
	wantFrags := []string{"{{.", "Key", "}}"}
	for i, p := range cf.parts {
		if p.text != wantFrags[i] {
			t.Errorf("parts[%d].text = %q, want %q", i, p.text, wantFrags[i])
		}
		if p.run != i {
			t.Errorf("parts[%d].run = %d, want %d", i, p.run, i)
		}
	}
	// Каждый фрагмент — неполный (ни один ран не содержит полный {{...}}).
	for i := range cf.parts {
		if !cf.parts[i].partial {
			t.Errorf("parts[%d].partial = false, want true (фрагмент неполного плейсхолдера)", i)
		}
	}
	// Суммарная ширина: {{. (1278) + Key (1650) + }} (1000) = 3928.
	expectWidth := textWidth("{{.") + textWidth("Key") + textWidth("}}")
	if cf.origWidth != expectWidth {
		t.Errorf("origWidth = %v, want %v", cf.origWidth, expectWidth)
	}
	if cf.value != "Bob" {
		t.Errorf("value = %q, want Bob", cf.value)
	}
}

func TestBuildCrossRunFieldsSingleRunSkipped(t *testing.T) {
	// Полные плейсхолдеры в одном ране не попадают в crossRunFields.
	stream := "BT /F1 12 Tf ({{.Key}} done) Tj ET"
	runs, err := textRuns(stream)
	if err != nil {
		t.Fatal(err)
	}
	cross := buildCrossRunFields(runs, map[string]string{"Key": "Bob"})
	if len(cross) != 0 {
		t.Errorf("len(cross) = %d, want 0 (within-run поле)", len(cross))
	}
}

func TestReplaceCrossRunFields(t *testing.T) {
	runs, err := textRuns(crossRunStream)
	if err != nil {
		t.Fatal(err)
	}
	cross := buildCrossRunFields(runs, map[string]string{"Key": "Bob"})

	edits, replaced, skipped := replaceCrossRunFields(runs, cross, false, nil, nil)
	out := applyEdits(crossRunStream, edits)
	if replaced != 1 {
		t.Errorf("replaced = %d, want 1", replaced)
	}
	if skipped != 0 {
		t.Errorf("skipped = %d, want 0", skipped)
	}

	// Ширины: {{.}=1278, Key=1650, }}=1000, сумма 3928.
	// Bob = 650+500+500 = 1650. delta = 3928-1650 = 2278.
	// Первый ран: [(Bob) -2278] TJ; второй: () -1650 Tj; третий: () -1000 Tj.
	delta := (textWidth("{{.") + textWidth("Key") + textWidth("}}")) - textWidth("Bob")
	wantFirst := "[(" + quoteString("Bob") + ") " + strconv.FormatFloat(-delta, 'g', -1, 64) + "] TJ"
	want := `BT /F1 12 Tf ` + wantFirst + ` ET BT /F1 12 Tf [() -` + strconv.FormatFloat(textWidth("Key"), 'g', -1, 64) + `] TJ ET BT /F1 12 Tf [() -` + strconv.FormatFloat(textWidth("}}"), 'g', -1, 64) + `] TJ ET`
	if out != want {
		t.Errorf("replaceCrossRunFields =\n%q\nwant\n%q", out, want)
	}
}

func TestReplaceCrossRunFieldsSkipsWideValue(t *testing.T) {
	runs, _ := textRuns(crossRunStream)
	// Широкое значение (10 букв * 650 = 6500) шире плейсхолдера (3928) — скип.
	wide := strings.Repeat("W", 10)
	cross := buildCrossRunFields(runs, map[string]string{"Key": wide})

	edits, replaced, skipped := replaceCrossRunFields(runs, cross, false, nil, nil)
	if applyEdits(crossRunStream, edits) != crossRunStream {
		t.Errorf("out = %q, want без изменений", applyEdits(crossRunStream, edits))
	}
	if replaced != 0 {
		t.Errorf("replaced = %d, want 0", replaced)
	}
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1", skipped)
	}
}

func TestReplaceCrossRunFieldsSkipsMissingKey(t *testing.T) {
	runs, _ := textRuns(crossRunStream)
	cross := buildCrossRunFields(runs, map[string]string{"Other": "x"})

	edits, replaced, skipped := replaceCrossRunFields(runs, cross, false, nil, nil)
	if applyEdits(crossRunStream, edits) != crossRunStream {
		t.Errorf("out = %q, want без изменений", applyEdits(crossRunStream, edits))
	}
	if replaced != 0 {
		t.Errorf("replaced = %d, want 0", replaced)
	}
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1", skipped)
	}
}

func TestReplaceCrossRunFieldsAllowWider(t *testing.T) {
	runs, _ := textRuns(crossRunStream)
	// Широкое значение (10 букв * 650 = 6500) шире плейсхолдера (3928),
	// но allowWider=true — заменяем с положительным кернингом.
	wide := strings.Repeat("W", 10)
	cross := buildCrossRunFields(runs, map[string]string{"Key": wide})

	edits, replaced, skipped := replaceCrossRunFields(runs, cross, true, nil, nil)
	out := applyEdits(crossRunStream, edits)
	if replaced != 1 {
		t.Errorf("replaced = %d, want 1", replaced)
	}
	if skipped != 0 {
		t.Errorf("skipped = %d, want 0", skipped)
	}
	if out == crossRunStream {
		t.Error("out = исходный поток, want замена")
	}
	if !strings.Contains(out, "WWWWWWWWWW") {
		t.Errorf("Stream не содержит значение: %q", out)
	}
}

func TestBuildStreamCrossRun(t *testing.T) {
	// Интеграция: textRuns → findFields → buildStream меняет {{.Key}} на Bob.
	runs, err := textRuns(crossRunStream)
	if err != nil {
		t.Fatal(err)
	}
	fields := findFields(crossRunStream, runs)
	out, err := buildStream(crossRunStream, runs, fields, map[string]string{"Key": "Bob"})
	if err != nil {
		t.Fatal(err)
	}

	delta := (textWidth("{{.") + textWidth("Key") + textWidth("}}")) - textWidth("Bob")
	wantFirst := "[(" + quoteString("Bob") + ") " + strconv.FormatFloat(-delta, 'g', -1, 64) + "] TJ"
	want := `BT /F1 12 Tf ` + wantFirst + ` ET BT /F1 12 Tf [() -` + strconv.FormatFloat(textWidth("Key"), 'g', -1, 64) + `] TJ ET BT /F1 12 Tf [() -` + strconv.FormatFloat(textWidth("}}"), 'g', -1, 64) + `] TJ ET`
	if out != want {
		t.Errorf("buildStream =\n%q\nwant\n%q", out, want)
	}
}

func TestReplaceAPICrossRun(t *testing.T) {
	// Публичный API: ReplaceParams / ReplaceResult с совместимой сигнатурой.
	res, err := Replace(&ReplaceParams{
		Stream: crossRunStream,
		Values: map[string]string{"Key": "Bob"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Replaced != 1 {
		t.Errorf("Replaced = %d, want 1", res.Replaced)
	}
	if res.Skipped != 0 {
		t.Errorf("Skipped = %d, want 0", res.Skipped)
	}
	if !strings.Contains(res.Stream, "Bob") {
		t.Errorf("Stream не содержит Bob: %q", res.Stream)
	}
	if !strings.Contains(res.Stream, "TJ") {
		t.Errorf("Stream не содержит TJ-кернинг: %q", res.Stream)
	}
	// Оригинал не должен содержать {{.Key}} целиком ни в одном ране.
	if strings.Contains(res.Stream, "{{.Key}}") {
		t.Errorf("Stream всё ещё содержит {{.Key}}: %q", res.Stream)
	}
}

func TestReplaceCrossRunWithAdjacentText(t *testing.T) {
	// Текст ПОСЛЕ плейсхолдера не должен 'поехать' из-за замены:
	// суммарный кернинг компенсирует разницу ширины.
	stream := `BT /F1 12 Tf ({{.) Tj ET BT /F1 12 Tf (Key) Tj ET BT /F1 12 Tf (}}) Tj ET BT /F1 12 Tf (Tail) Tj ET`
	runs, err := textRuns(stream)
	if err != nil {
		t.Fatal(err)
	}
	out, err := buildStream(stream, runs, findFields(stream, runs), map[string]string{"Key": "Bob"})
	if err != nil {
		t.Fatal(err)
	}
	// Хвост (Tail) Tj должен остаться нетронутым.
	if !strings.Contains(out, "(Tail) Tj") {
		t.Errorf("хвост переехал: %q", out)
	}
}

// stubCodec — фиктивный кодек для проверки переключения рана на внедрённый
// шрифт: содержимое в hex-форме, заданные ширины, новое имя ресурса /TF1.
type stubCodec struct{ w, nw float64 }

func (c stubCodec) Encode(s string) string    { return s + "~" }
func (c stubCodec) Hex() bool                 { return true }
func (c stubCodec) Width(s string) float64    { return c.w }
func (c stubCodec) NewWidth(s string) float64 { return c.nw }
func (c stubCodec) FontName() string          { return "/TF1" }

func TestReplaceWithCodecHex(t *testing.T) {
	// Ран не покрывает значение (провайдер возвращает кодек) — шрифт рана
	// переключается на /TF1, значение вставляется hex-литералом <...>,
	// дельта компенсируется кернингом (ширины из кодеков).
	stream := "BT /F1 12 Tf ({{.Name}}) Tj ET"
	res, err := Replace(&ReplaceParams{
		Stream:     stream,
		Values:     map[string]string{"Name": "\u041f\u0440\u0438\u0432\u0435\u0442"},
		AllowWider: true,
		Codec: func(font, text string) RunCodec {
			if font != "/F1" {
				t.Errorf("provider font = %q, want /F1", font)
			}
			return stubCodec{w: 3000, nw: 1000}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Replaced != 1 {
		t.Errorf("Replaced = %d, want 1", res.Replaced)
	}
	// 3000-1000 = 2000 → TJ с кернингом -2000.
	if !strings.Contains(res.Stream, "/TF1 12 Tf") {
		t.Errorf("Tf не переименован: %q", res.Stream)
	}
	if !strings.Contains(res.Stream, "[<") || !strings.Contains(res.Stream, "> -2000] TJ") {
		t.Errorf("нет hex-литерала TJ: %q", res.Stream)
	}
	if strings.Contains(res.Stream, ") Tj") {
		t.Errorf("остался строковый оператор: %q", res.Stream)
	}
	// Hex-содержимое — это Encode (utf8 + суффикс stub).
	hex := hexEncode([]byte("\u041f\u0440\u0438\u0432\u0435\u0442~"))
	if !strings.Contains(res.Stream, hex) {
		t.Errorf("нет hex-байтов Encode: %q", res.Stream)
	}
	t.Logf("stream: %s", res.Stream)
}

func TestReplaceWithCodecSkipsWideByCodec(t *testing.T) {
	// Ширины считаются кодеками: значение по метрикам внедряемого шрифта
	// (nw=5000) шире области плейсхолдера (w=3000) → замена отменяется.
	stream := "BT /F1 12 Tf ({{.Name}}) Tj ET"
	res, err := Replace(&ReplaceParams{
		Stream: stream,
		Values: map[string]string{"Name": "\u041f"},
		Codec:  func(font, text string) RunCodec { return stubCodec{w: 3000, nw: 5000} },
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Replaced != 0 || res.Skipped != 1 {
		t.Errorf("Replaced/Skipped = %d/%d, want 0/1", res.Replaced, res.Skipped)
	}
	if res.Stream != stream {
		t.Errorf("поток изменён при запрете широких значений: %q", res.Stream)
	}
}

func TestReplaceNoCodecKeepsLegacy(t *testing.T) {
	// Провайдер отсутствует — поведение прежнее: литерал в исходном шрифте
	// /F1, строковая форма с компенсацией ширины при расхождении метрик.
	stream := "BT /F1 12 Tf ({{.Name}}) Tj ET"
	res, err := Replace(&ReplaceParams{Stream: stream, Values: map[string]string{"Name": "Bob"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Stream, "/F1 12 Tf") {
		t.Errorf("шрифт изменён при отсутствии провайдера: %q", res.Stream)
	}
	if !strings.Contains(res.Stream, "(Bob)") {
		t.Errorf("значение не вставлено как строка: %q", res.Stream)
	}
	if strings.Contains(res.Stream, "<") {
		t.Errorf("появился hex-литерал: %q", res.Stream)
	}
}

func TestReplaceCodecNilMeansNoSwitch(t *testing.T) {
	// Провайдер вернул nil (шрифт покрывает текст) — ран не переключаем.
	calls := 0
	res, err := Replace(&ReplaceParams{
		Stream: "BT /F1 12 Tf ({{.A}}) Tj ET",
		Values: map[string]string{"A": "x"},
		Codec: func(font, text string) RunCodec {
			calls++
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Replaced != 1 {
		t.Errorf("Replaced = %d, want 1", res.Replaced)
	}
	if calls == 0 {
		t.Error("провайдер не вызывался")
	}
	if strings.Contains(res.Stream, "/TF") {
		t.Errorf("ран переключён при nil-кодеке: %q", res.Stream)
	}
}
