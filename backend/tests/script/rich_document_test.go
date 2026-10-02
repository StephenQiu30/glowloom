package script_test

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/script/adapter/extract"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

func TestRichDocumentHTMLPreservesSourceEditingSemantics(t *testing.T) {
	input := `<h2 style="text-align:center">序章</h2><p><strong>人</strong><em>😀</em><u>é</u><s>旧</s><a href="https://example.test/a" target="_blank" rel="noopener noreferrer">链接</a><span style="color:rgb(217, 119, 6)">色</span><mark data-color="#fef3c7" style="background-color:#fef3c7;color:inherit">光</mark><br>尾</p><ol start="3"><li><p>一</p><ul><li><p>内</p></li></ul></li><li><p>二</p></li></ol><blockquote><p>引</p></blockquote><pre><code class="language-go">a\nb</code></pre><hr><p></p>`
	doc, err := extract.HTML(input)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := doc.PlainText()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plain, "人😀é旧链接色光\n尾") || !strings.Contains(plain, "一\n\n内\n二") {
		t.Fatalf("lost text or explicit line structure: %q", plain)
	}
	if doc.Content[0].Attrs == nil || doc.Content[0].Attrs.Level != 2 || doc.Content[0].Attrs.Align != "center" {
		t.Fatalf("lost heading alignment: %+v", doc.Content[0])
	}
	if doc.Content[2].Type != "orderedList" || doc.Content[2].Attrs.Start == nil || *doc.Content[2].Attrs.Start != 3 || doc.Content[5].Type != "horizontalRule" || len(doc.Content[6].Content) != 0 {
		t.Fatalf("lost ordered list or zero-width nodes: %+v", doc.Content)
	}
	encoded, err := doc.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := domain.DecodeRichDocument(encoded)
	if err != nil || !reflect.DeepEqual(doc, decoded) {
		t.Fatalf("rich semantic round trip: %v", err)
	}
	for _, mark := range []string{"bold", "italic", "underline", "strike", "link", "textColor", "highlight"} {
		if !strings.Contains(string(encoded), `"type":"`+mark+`"`) {
			t.Fatalf("lost %s", mark)
		}
	}
}

func TestRichDocumentClosedBoundaryRejectsSilentLoss(t *testing.T) {
	for _, input := range []string{
		`<h4>unsupported heading</h4>`, `<p><img src="https://example.test/a.png"></p>`, `<p onclick="x()">x</p>`,
		`<p style="font-size:50px">x</p>`, `<p><a href="javascript:alert(1)">x</a></p>`, `<iframe>body</iframe>`,
	} {
		if _, err := extract.HTML(input); !errors.Is(err, domain.ErrInvalidDocument) {
			t.Errorf("accepted lossy HTML %q: %v", input, err)
		}
	}
	for _, input := range []string{
		`{"type":"doc","content":[],"unknown":true}`, `{"type":"doc","content":[{"type":"paragraph","attrs":{"unknown":"x"}}]}`,
		`{"type":"doc","content":[{"type":"text","text":"x"}]}`, `{"type":"doc","content":[{"type":"heading","attrs":{"level":4}}]}`,
		`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"\ud800"}]}]}`,
		`{"type":"paragraph","type":"doc","content":[]}`,
	} {
		if _, err := domain.DecodeRichDocument([]byte(input)); !errors.Is(err, domain.ErrInvalidDocument) {
			t.Errorf("accepted unknown or invalid rich input %s: %v", input, err)
		}
	}
}

func TestRichDocumentBrowserHighlightColorNormalization(t *testing.T) {
	a, err := extract.HTML(`<p><mark data-color="#fef3c7" style="background-color: rgb(254, 243, 199); color: inherit;">光</mark></p>`)
	if err != nil {
		t.Fatal(err)
	}
	b, err := extract.HTML(`<p><mark data-color="#fef3c7" style="background-color:#fef3c7;color:inherit">光</mark></p>`)
	if err != nil {
		t.Fatal(err)
	}
	_, ha, err := a.Hashes()
	if err != nil {
		t.Fatal(err)
	}
	_, hb, err := b.Hashes()
	if err != nil || ha != hb {
		t.Fatalf("browser-normalized color changed semantic SHA: %s %s %v", ha, hb, err)
	}
}

func TestRichDocumentScalarNormalizationAndFormatHashes(t *testing.T) {
	a, err := extract.HTML("<p> 😀é\r\n中 </p><p></p><p>終<br>点</p>")
	if err != nil {
		t.Fatal(err)
	}
	plain, err := a.PlainText()
	if err != nil || plain != " 😀é\n中 \n\n\n\n終\n点" {
		t.Fatalf("unexpected normalization: %q %v", plain, err)
	}
	part, err := domain.ScalarSlice(plain, 1, 4)
	if err != nil || part != "😀é" {
		t.Fatalf("UTF16/byte coordinate leak: %q %v", part, err)
	}
	b, err := extract.HTML("<p><strong> 😀é\r\n中 </strong></p><p></p><p>終<br>点</p>")
	if err != nil {
		t.Fatal(err)
	}
	pa, ra, err := a.Hashes()
	if err != nil {
		t.Fatal(err)
	}
	pb, rb, err := b.Hashes()
	if err != nil || pa != pb || ra == rb {
		t.Fatalf("format change mixed plain/rich hashes: %s %s %s %s %v", pa, pb, ra, rb, err)
	}
	if _, err := domain.ScalarSlice(plain, 4, 1); !errors.Is(err, domain.ErrInvalidSpan) {
		t.Fatal("accepted reverse scalar span")
	}
}

func TestRichDocumentBudgetsAndCodeWhitespace(t *testing.T) {
	input := `{"type":"doc","content":[{"type":"codeBlock","attrs":{"language":"go"},"content":[{"type":"text","text":"  a\r\nb\n"}]}]}`
	doc, err := domain.DecodeRichDocument([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := doc.PlainText()
	if err != nil || plain != "  a\nb\n" {
		t.Fatalf("code whitespace changed: %q %v", plain, err)
	}
	deep := `<p>x</p>`
	for range 65 {
		deep = "<blockquote>" + deep + "</blockquote>"
	}
	if _, err := extract.HTML(deep); !errors.Is(err, domain.ErrInvalidDocument) {
		t.Fatalf("unbounded rich depth: %v", err)
	}
	if _, err := extract.HTML("<p>" + strings.Repeat("a", domain.MaxScalarCount+1) + "</p>"); !errors.Is(err, domain.ErrInvalidDocument) {
		t.Fatalf("unbounded rich text: %v", err)
	}
}

func TestRichDocumentExplicitZeroListStartAndEmptyAttributeCanonicalization(t *testing.T) {
	input := []byte(`{"type":"doc","attrs":{},"content":[{"type":"orderedList","attrs":{"start":0},"content":[{"type":"listItem","attrs":{},"content":[{"type":"paragraph","attrs":{},"content":[{"type":"text","attrs":{},"text":"正文","marks":[{"type":"bold","attrs":{}}]}]}]}]}]}`)
	doc, err := domain.DecodeRichDocument(input)
	if err != nil {
		t.Fatal(err)
	}
	data, err := doc.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"start":0`)) || bytes.Contains(data, []byte(`"attrs":{}`)) {
		t.Fatal("explicit zero or empty canonical attrs", string(data))
	}
}
