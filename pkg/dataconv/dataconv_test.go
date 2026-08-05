package dataconv

import (
	"bytes"
	"encoding/json"
	"testing"
)

// --- xmlToMap ---

func TestXmlToMap(t *testing.T) {
	input := []byte(`<root><name>Test</name><value>42</value></root>`)
	m, err := xmlToMap(bytes.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if m["name"] != "Test" {
		t.Errorf("name = %v, want Test", m["name"])
	}
	if m["value"] != "42" {
		t.Errorf("value = %v, want 42", m["value"])
	}
}

func TestXmlToMapNested(t *testing.T) {
	input := []byte(`<root><person><name>Ivan</name><age>30</age></person></root>`)
	m, err := xmlToMap(bytes.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	person, ok := m["person"].(map[string]any)
	if !ok {
		t.Fatalf("person should be map, got %T", m["person"])
	}
	if person["name"] != "Ivan" {
		t.Errorf("person.name = %v, want Ivan", person["name"])
	}
	if person["age"] != "30" {
		t.Errorf("person.age = %v, want 30", person["age"])
	}
}

func TestXmlToMapRepeated(t *testing.T) {
	input := []byte(`<root><item>1</item><item>2</item><item>3</item></root>`)
	m, err := xmlToMap(bytes.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	items, ok := m["item"].([]any)
	if !ok {
		t.Fatalf("item should be []any, got %T", m["item"])
	}
	if len(items) != 3 {
		t.Fatalf("len(items) = %d, want 3", len(items))
	}
	for i, want := range []string{"1", "2", "3"} {
		if items[i] != want {
			t.Errorf("items[%d] = %v, want %s", i, items[i], want)
		}
	}
}

func TestXmlToMapTextOnly(t *testing.T) {
	// <root>Hello World</root> — root без дочерних элементов, только текст.
	// xmlToMap вернёт ошибку "root must be an object", т.к. readValueEOF
	// вернёт строку, а не map.
	input := []byte(`<root>Hello World</root>`)
	_, err := xmlToMap(bytes.NewReader(input))
	if err == nil {
		t.Error("expected error for text-only root")
	}
}

func TestXmlToMapEmptyElement(t *testing.T) {
	input := []byte(`<root><empty/></root>`)
	m, err := xmlToMap(bytes.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	// empty/ — пустой элемент становится map[string]any{}
	empty, ok := m["empty"].(map[string]any)
	if !ok {
		// Если xml.Decoder вернул строку — это тоже допустимо
		if _, ok2 := m["empty"].(string); !ok2 {
			t.Fatalf("empty should be map or string, got %T", m["empty"])
		}
		return
	}
	if len(empty) != 0 {
		t.Errorf("empty map should be empty, got %d entries", len(empty))
	}
}

func TestXmlToMapWhitespaceTrim(t *testing.T) {
	input := []byte(`<root>
  <name>  Ivan  </name>
</root>`)
	m, err := xmlToMap(bytes.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if m["name"] != "Ivan" {
		t.Errorf("name = %q, want Ivan (whitespace trimmed)", m["name"])
	}
}

func TestXmlToMapCyrillic(t *testing.T) {
	input := []byte(`<root><name>Иван</name><desc>Текст с пробелами</desc></root>`)
	m, err := xmlToMap(bytes.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if m["name"] != "Иван" {
		t.Errorf("name = %v, want Иван", m["name"])
	}
	if m["desc"] != "Текст с пробелами" {
		t.Errorf("desc = %v, want Текст с пробелами", m["desc"])
	}
}

func TestXmlToMapInvalidXml(t *testing.T) {
	input := []byte(`not xml {{{`)
	_, err := xmlToMap(bytes.NewReader(input))
	if err == nil {
		t.Error("expected error for invalid XML")
	}
}

func TestXmlToMapEmptyRoot(t *testing.T) {
	input := []byte(`<root></root>`)
	// readValueEnd с пустыми children и пустым текстом возвращает "" (строку),
	// а не map — xmlToMap вернёт ошибку "root must be an object"
	_, err := xmlToMap(bytes.NewReader(input))
	if err == nil {
		t.Error("expected error for empty root")
	}
}

// --- DataMerge ---

func TestDataMerge(t *testing.T) {
	data := [][]byte{
		[]byte(`<root><name>Ivan</name><age>30</age></root>`),
		[]byte(`<root><city>Moscow</city><age>31</age></root>`),
	}
	merged, err := DataMerge(data)
	if err != nil {
		t.Fatal(err)
	}

	var m map[string]any
	if err := json.Unmarshal(merged, &m); err != nil {
		t.Fatal(err)
	}

	if m["name"] != "Ivan" {
		t.Errorf("name = %v, want Ivan", m["name"])
	}
	if m["city"] != "Moscow" {
		t.Errorf("city = %v, want Moscow", m["city"])
	}
	// Второе значение переопределяет первое
	if m["age"] != "31" {
		t.Errorf("age = %v, want 31 (overridden)", m["age"])
	}
}

func TestDataMergeNoData(t *testing.T) {
	_, err := DataMerge(nil)
	if err == nil {
		t.Error("expected error for nil data")
	}

	_, err = DataMerge([][]byte{})
	if err == nil {
		t.Error("expected error for empty data")
	}
}

func TestDataMergeSingleFile(t *testing.T) {
	data := [][]byte{
		[]byte(`<root><name>Single</name></root>`),
	}
	merged, err := DataMerge(data)
	if err != nil {
		t.Fatal(err)
	}

	var m map[string]any
	if err := json.Unmarshal(merged, &m); err != nil {
		t.Fatal(err)
	}
	if m["name"] != "Single" {
		t.Errorf("name = %v, want Single", m["name"])
	}
}

func TestDataMergeDeepMerge(t *testing.T) {
	data := [][]byte{
		[]byte(`<root><config><a>1</a><b>old</b></config></root>`),
		[]byte(`<root><config><b>new</b><c>2</c></config></root>`),
	}
	merged, err := DataMerge(data)
	if err != nil {
		t.Fatal(err)
	}

	var m map[string]any
	if err := json.Unmarshal(merged, &m); err != nil {
		t.Fatal(err)
	}

	config, ok := m["config"].(map[string]any)
	if !ok {
		t.Fatalf("config should be map, got %T", m["config"])
	}
	if config["a"] != "1" {
		t.Errorf("config.a = %v, want 1 (preserved)", config["a"])
	}
	if config["b"] != "new" {
		t.Errorf("config.b = %v, want new (overridden)", config["b"])
	}
	if config["c"] != "2" {
		t.Errorf("config.c = %v, want 2 (added)", config["c"])
	}
}

func TestDataMergeWithActsigners(t *testing.T) {
	data := [][]byte{
		[]byte(`<root><ActSigner1Position>Директор</ActSigner1Position><ActSigner1FIO>Иванов Иван</ActSigner1FIO></root>`),
	}
	merged, err := DataMerge(data)
	if err != nil {
		t.Fatal(err)
	}

	var m map[string]any
	if err := json.Unmarshal(merged, &m); err != nil {
		t.Fatal(err)
	}

	signers, ok := m["Actsigners"].([]any)
	if !ok {
		t.Fatal("Actsigners should exist")
	}
	if len(signers) != 1 {
		t.Fatalf("len(Actsigners) = %d, want 1", len(signers))
	}
}

func TestDataMergeMultipleSigners(t *testing.T) {
	data := [][]byte{
		[]byte(`<root><ActSigner1Position>Директор</ActSigner1Position><ActSigner1FIO>Иванов Иван Иванович</ActSigner1FIO><ActSigner2Position>Бухгалтер</ActSigner2Position><ActSigner2FIO>Петров Пётр Петрович</ActSigner2FIO></root>`),
	}
	merged, err := DataMerge(data)
	if err != nil {
		t.Fatal(err)
	}

	var m map[string]any
	if err := json.Unmarshal(merged, &m); err != nil {
		t.Fatal(err)
	}

	signers, ok := m["Actsigners"].([]any)
	if !ok {
		t.Fatal("Actsigners should exist")
	}
	if len(signers) != 2 {
		t.Fatalf("len(Actsigners) = %d, want 2", len(signers))
	}

	s1, ok := signers[0].(map[string]any)
	if !ok {
		t.Fatalf("signer[0] should be map, got %T", signers[0])
	}
	if s1["Position"] != "Директор" {
		t.Errorf("signer[0].Position = %v, want Директор", s1["Position"])
	}
	if s1["FIO"] != "Иванов Иван Иванович" {
		t.Errorf("signer[0].FIO = %v, want Иванов Иван Иванович", s1["FIO"])
	}
	if s1["Sign"] != "Иванов.png" {
		t.Errorf("signer[0].Sign = %v, want Иванов.png", s1["Sign"])
	}

	s2, ok := signers[1].(map[string]any)
	if !ok {
		t.Fatalf("signer[1] should be map, got %T", signers[1])
	}
	if s2["Sign"] != "Петров.png" {
		t.Errorf("signer[1].Sign = %v, want Петров.png", s2["Sign"])
	}
}

func TestDataMergeEmptySigners(t *testing.T) {
	data := [][]byte{
		[]byte(`<root><name>Test</name></root>`),
	}
	merged, err := DataMerge(data)
	if err != nil {
		t.Fatal(err)
	}

	var m map[string]any
	if err := json.Unmarshal(merged, &m); err != nil {
		t.Fatal(err)
	}

	if _, ok := m["Actsigners"]; ok {
		t.Error("Actsigners should not be added when no signer fields exist")
	}
}

func TestDataMergeSignerWithEmptyFIO(t *testing.T) {
	data := [][]byte{
		[]byte(`<root><ActSigner1Position>Директор</ActSigner1Position><ActSigner1FIO></ActSigner1FIO></root>`),
	}
	merged, err := DataMerge(data)
	if err != nil {
		t.Fatal(err)
	}

	var m map[string]any
	if err := json.Unmarshal(merged, &m); err != nil {
		t.Fatal(err)
	}

	signers, ok := m["Actsigners"].([]any)
	if !ok {
		t.Fatal("Actsigners should exist")
	}
	if len(signers) != 1 {
		t.Fatalf("len(Actsigners) = %d, want 1", len(signers))
	}

	s1, ok := signers[0].(map[string]any)
	if !ok {
		t.Fatalf("signer[0] should be map, got %T", signers[0])
	}
	if s1["Sign"] != "" {
		t.Errorf("signer[0].Sign = %q, want empty for empty FIO", s1["Sign"])
	}
}

// --- mergeMaps ---

func TestMergeMapsDeep(t *testing.T) {
	dst := map[string]any{
		"a": map[string]any{
			"x": 1,
			"y": 2,
		},
		"b": "old",
	}
	src := map[string]any{
		"a": map[string]any{
			"z": 3,
		},
		"b": "new",
		"c": "new2",
	}
	mergeMaps(dst, src)

	a := dst["a"].(map[string]any)
	if a["x"] != 1 {
		t.Errorf("a.x = %v, want 1", a["x"])
	}
	if a["y"] != 2 {
		t.Errorf("a.y = %v, want 2", a["y"])
	}
	if a["z"] != 3 {
		t.Errorf("a.z = %v, want 3", a["z"])
	}
	if dst["b"] != "new" {
		t.Errorf("b = %v, want new (overridden)", dst["b"])
	}
	if dst["c"] != "new2" {
		t.Errorf("c = %v, want new2", dst["c"])
	}
}

func TestMergeMapsSliceOverride(t *testing.T) {
	dst := map[string]any{
		"items": []any{"a", "b"},
	}
	src := map[string]any{
		"items": []any{"c"},
	}
	mergeMaps(dst, src)
	items := dst["items"].([]any)
	if len(items) != 1 || items[0] != "c" {
		t.Errorf("items = %v, want [c] (overridden, not merged)", items)
	}
}

func TestMergeMapsEmptyMaps(t *testing.T) {
	dst := map[string]any{}
	src := map[string]any{}
	mergeMaps(dst, src)
	if len(dst) != 0 {
		t.Errorf("expected empty dst, got %d entries", len(dst))
	}
}

func TestMergeMapsNilSrc(t *testing.T) {
	dst := map[string]any{
		"a": "value",
	}
	mergeMaps(dst, nil)
	if dst["a"] != "value" {
		t.Errorf("dst should be unchanged, got %v", dst["a"])
	}
}

// --- collapseChildren ---

func TestCollapseChildrenSingleString(t *testing.T) {
	children := map[string]any{"text": "hello"}
	result := collapseChildren(children)
	// collapseSingleChild не обрабатывает string, падает в return children
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", result)
	}
	if m["text"] != "hello" {
		t.Errorf("text = %v, want hello", m["text"])
	}
}

func TestCollapseChildrenSingleMap(t *testing.T) {
	children := map[string]any{"item": map[string]any{"key": "val"}}
	result := collapseChildren(children)
	// Одиночная карта оборачивается в список
	list, ok := result.([]any)
	if !ok {
		t.Fatalf("collapse single map should return []any, got %T", result)
	}
	if len(list) != 1 {
		t.Fatalf("len(list) = %d, want 1", len(list))
	}
	item, ok := list[0].(map[string]any)
	if !ok {
		t.Fatalf("list[0] should be map, got %T", list[0])
	}
	if item["key"] != "val" {
		t.Errorf("item.key = %v, want val", item["key"])
	}
}

func TestCollapseChildrenMultiple(t *testing.T) {
	children := map[string]any{
		"a": "1",
		"b": "2",
	}
	result := collapseChildren(children)
	if _, ok := result.(map[string]any); !ok {
		t.Error("collapse multiple children should return map")
	}
}

func TestCollapseChildrenEmpty(t *testing.T) {
	children := map[string]any{}
	result := collapseChildren(children)
	if _, ok := result.(map[string]any); !ok {
		t.Error("collapse empty children should return map")
	}
}

func TestCollapseChildrenSingleList(t *testing.T) {
	children := map[string]any{"items": []any{"a", "b"}}
	result := collapseChildren(children)
	list, ok := result.([]any)
	if !ok {
		t.Fatalf("collapse single list should return []any, got %T", result)
	}
	if len(list) != 2 {
		t.Errorf("len(list) = %d, want 2", len(list))
	}
}

// --- postProcessMerge ---

func TestPostProcessMergeMultipleSigners(t *testing.T) {
	data := map[string]any{
		"ActSigner1Position": "Директор",
		"ActSigner1FIO":      "Иванов Иван Иванович",
		"ActSigner2Position": "Бухгалтер",
		"ActSigner2FIO":      "Петров Пётр Петрович",
	}
	postProcessMerge(data)

	signers, ok := data["Actsigners"].([]any)
	if !ok {
		t.Fatal("Actsigners should exist")
	}
	if len(signers) != 2 {
		t.Fatalf("len(Actsigners) = %d, want 2", len(signers))
	}

	s1, ok := signers[0].(map[string]any)
	if !ok {
		t.Fatalf("signer[0] should be map, got %T", signers[0])
	}
	if s1["Position"] != "Директор" {
		t.Errorf("signer[0].Position = %v, want Директор", s1["Position"])
	}
	if s1["FIO"] != "Иванов Иван Иванович" {
		t.Errorf("signer[0].FIO = %v, want Иванов Иван Иванович", s1["FIO"])
	}
	if s1["Sign"] != "Иванов.png" {
		t.Errorf("signer[0].Sign = %v, want Иванов.png", s1["Sign"])
	}

	s2, ok := signers[1].(map[string]any)
	if !ok {
		t.Fatalf("signer[1] should be map, got %T", signers[1])
	}
	if s2["Sign"] != "Петров.png" {
		t.Errorf("signer[1].Sign = %v, want Петров.png", s2["Sign"])
	}
}

func TestPostProcessMergeSingleSigner(t *testing.T) {
	data := map[string]any{
		"ActSigner1Position": "Директор",
		"ActSigner1FIO":      "Иванов Иван",
	}
	postProcessMerge(data)

	signers, ok := data["Actsigners"].([]any)
	if !ok {
		t.Fatal("Actsigners should exist")
	}
	if len(signers) != 1 {
		t.Fatalf("len(Actsigners) = %d, want 1", len(signers))
	}
}

func TestPostProcessMergeNoSigners(t *testing.T) {
	data := map[string]any{
		"name": "Test",
	}
	postProcessMerge(data)
	if _, ok := data["Actsigners"]; ok {
		t.Error("Actsigners should not be added when no signer fields exist")
	}
}

func TestPostProcessMergePartialSigner(t *testing.T) {
	// Только Position, без FIO
	data := map[string]any{
		"ActSigner1Position": "Директор",
	}
	postProcessMerge(data)

	signers, ok := data["Actsigners"].([]any)
	if !ok {
		t.Fatal("Actsigners should exist")
	}
	if len(signers) != 1 {
		t.Fatalf("len(Actsigners) = %d, want 1", len(signers))
	}
	s1, ok := signers[0].(map[string]any)
	if !ok {
		t.Fatalf("signer[0] should be map, got %T", signers[0])
	}
	if s1["Position"] != "Директор" {
		t.Errorf("signer[0].Position = %v, want Директор", s1["Position"])
	}
	if s1["FIO"] != "" {
		t.Errorf("signer[0].FIO = %q, want empty", s1["FIO"])
	}
	if s1["Sign"] != "" {
		t.Errorf("signer[0].Sign = %q, want empty for missing FIO", s1["Sign"])
	}
}

// --- Benchmarks ---

func BenchmarkDataMerge(b *testing.B) {
	data := [][]byte{
		[]byte(`<root><name>Ivan</name><age>30</age><city>Moscow</city></root>`),
		[]byte(`<root><position>Director</position><sign>true</sign></root>`),
	}
	b.ResetTimer()
	for range b.N {
		_, _ = DataMerge(data)
	}
}

func BenchmarkDataMergeLarge(b *testing.B) {
	data := [][]byte{
		[]byte(`<root><ActSigner1Position>Директор</ActSigner1FIO>Иванов Иван Иванович</ActSigner1FIO><ActSigner2Position>Бухгалтер</ActSigner2FIO>Петров Пётр Петрович</ActSigner2FIO><name>Test</name><city>Moscow</city></root>`),
		[]byte(`<root><config><a>1</a><b>2</b><c>3</c></config></root>`),
		[]byte(`<root><extra>data</extra></root>`),
	}
	b.ResetTimer()
	for range b.N {
		_, _ = DataMerge(data)
	}
}
