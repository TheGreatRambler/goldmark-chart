package goldmark_chart

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// Chart node used instead of handling `ast.FencedCodeBlock` instances
type ChartBlock struct {
	ast.BaseBlock
}

var KindChartBlock = ast.NewNodeKind("ChartBlock")

func (n *ChartBlock) Kind() ast.NodeKind {
	return KindChartBlock
}

func (n *ChartBlock) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

type Transformer struct {
}

var VIS_LANG = []byte("vis")

// Transform code blocks into chart blocks
func (s *Transformer) Transform(doc *ast.Document, reader text.Reader, pctx parser.Context) {
	var blocks []*ast.FencedCodeBlock

	// Collect all chart blocks
	ast.Walk(doc, func(node ast.Node, enter bool) (ast.WalkStatus, error) {
		if !enter {
			return ast.WalkContinue, nil
		}

		cb, ok := node.(*ast.FencedCodeBlock)
		if !ok {
			return ast.WalkContinue, nil
		}

		lang := cb.Language(reader.Source())
		if !bytes.Equal(lang, VIS_LANG) {
			return ast.WalkContinue, nil
		}

		blocks = append(blocks, cb)
		return ast.WalkContinue, nil
	})

	// Nothing to do
	if len(blocks) == 0 {
		return
	}

	// Modify those blocks in place
	for _, cb := range blocks {
		b := new(ChartBlock)
		b.SetLines(cb.Lines())

		parent := cb.Parent()
		if parent != nil {
			parent.ReplaceChild(parent, cb, b)
		}
	}
}

type HTMLRenderer struct {
	// Options
}

func (r *HTMLRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(KindChartBlock, r.Render)
}

type RenderPoint struct {
	Key   string
	Value float64
}

type RenderChartData struct {
	Type        string
	Points      []RenderPoint
	KeysNumeric bool
}

func ParseChartData(input string) (RenderChartData, error) {
	lines := strings.Split(strings.TrimSpace(input), "\n")
	var chart_type string
	var data_lines []string
	in_data := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "layout:") {
			chart_type = strings.TrimSpace(strings.TrimPrefix(line, "layout:"))
		} else if strings.HasPrefix(line, "data:") {
			in_data = true
			// add everything after `data:` in this line (if `[ ...` is on same line)
			if i := strings.Index(line, "["); i != -1 {
				data_lines = append(data_lines, line[i:])
			}
		} else if in_data {
			data_lines = append(data_lines, line)
		}
	}

	if chart_type == "" {
		return RenderChartData{}, errors.New("layout not found")
	}
	if len(data_lines) == 0 {
		return RenderChartData{}, errors.New("data not found")
	}

	// Join and make sure JSON syntax is valid
	data_str := strings.Join(data_lines, "\n")
	data_str = strings.TrimSuffix(data_str, "]")
	data_str = strings.TrimSpace(data_str)
	if !strings.HasPrefix(data_str, "[") {
		data_str = "[" + data_str
	}
	if !strings.HasSuffix(data_str, "]") {
		data_str = data_str + "]"
	}

	// Replace keys like `key:` and `value:` with JSON keys `"key":`
	data_str = strings.ReplaceAll(data_str, "key:", `"key":`)
	data_str = strings.ReplaceAll(data_str, "value:", `"value":`)
	// Quote unquoted object keys and make valid JSON
	data_str = strings.ReplaceAll(data_str, "'", `"`)
	// Ensure trailing commas don't break parsing
	data_str = strings.ReplaceAll(data_str, ", }", "}")
	data_str = strings.ReplaceAll(data_str, ",]", "]")

	var points []RenderPoint
	if err := json.Unmarshal([]byte(data_str), &points); err != nil {
		return RenderChartData{}, fmt.Errorf("failed to parse data: %w", err)
	}

	keys_numeric := true
	for _, p := range points {
		if _, err := fmt.Sscanf(p.Key, "%f", new(float64)); err != nil {
			keys_numeric = false
			break
		}
	}

	return RenderChartData{
		Type:        chart_type,
		Points:      points,
		KeysNumeric: keys_numeric,
	}, nil
}

// BuildChartJS returns a <script> tag string for Chart.js.
func BuildChartJS(div_id string, cd RenderChartData) string {
	// Normalize type
	t := strings.ToLower(strings.TrimSpace(cd.Type))
	switch t {
	case "bar", "line", "pie":
	default:
		t = "bar"
	}

	// Prepare labels and values
	labels := make([]string, len(cd.Points))
	values := make([]float64, len(cd.Points))
	for i, p := range cd.Points {
		labels[i] = p.Key
		values[i] = p.Value
	}
	labels_json, _ := json.Marshal(labels)
	values_json, _ := json.Marshal(values)

	// Dataset changes a bit for pie (needs per-slice colors; no y-scale)
	dataset := fmt.Sprintf(`{
      label: %q,
      data: %s,
      borderWidth: 1
    }`, cases.Title(language.English).String(t), values_json)

	options := `{}`

	// Generate options
	if t == "pie" {
		dataset = fmt.Sprintf(`{
    		label: %q,
    		data: %s,
    		backgroundColor: labels.map((_, i) => 'hsl(' + (i * 360 / Math.max(1, labels.length)) + ',70%%,60%%)'),
    		borderWidth: 1
    	}`, "Series", values_json)
		// pie: no scales
		options = `{}`
	} else {
		// bar/line: keep a simple y scale
		options = `{
      		scales: {
      			y: { beginAtZero: true }
      		}
    	}`
	}

	return fmt.Sprintf(`
	<script>
		(function () {
			const ctx = document.getElementById("%s").getContext("2d");
			const labels = %s;
			const data = {
				labels,
				datasets: [%s]
			};
			const config = {
				type: %q,
				data,
				options: %s
			};
			new Chart(ctx, config);
		})();
	</script>`, div_id, labels_json, dataset, t, options)
}

// Finally render
func (r *HTMLRenderer) Render(w util.BufWriter, src []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ChartBlock)
	if !entering {
		// We do not do anything at the closing tag
		return ast.WalkContinue, nil
	}

	w.WriteString(`<div class="d2">`)

	input_b := []byte{}
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		line := lines.At(i)
		input_b = append(input_b, line.Value(src)...)
	}

	if len(input_b) == 0 {
		return ast.WalkContinue, nil
	}

	out := []byte{}

	// Generate ID for div
	div_id_hash := sha256.New()
	div_id_hash.Write(input_b)
	div_id := hex.EncodeToString(div_id_hash.Sum(nil))

	out = append(out, []byte(fmt.Sprintf(`<div id="%s" width="100%%"></div>`, div_id))...)

	// Get chart data
	chart_data, err := ParseChartData(string(input_b))
	if err != nil {
		// Currently just ignore
		return ast.WalkContinue, err
	}

	out = append(out, []byte(BuildChartJS(div_id, chart_data))...)

	_, err = w.Write(out)
	return ast.WalkContinue, err
}

// Instance used as extension
type Chart struct {
	// Options
}

func (e *Chart) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(parser.WithASTTransformers(
		util.Prioritized(&Transformer{}, 100),
	))
	m.Renderer().AddOptions(renderer.WithNodeRenderers(
		util.Prioritized(&HTMLRenderer{
			// Options
		}, 0),
	))
}
