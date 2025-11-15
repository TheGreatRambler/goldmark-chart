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
	Key   interface{} `json:"key,string"`
	Value float64
}

type RenderChartData struct {
	Type        string
	Height      string
	Label       string
	Title       string
	Points      []RenderPoint
	KeysNumeric bool
	Color       string
}

func ParseChartData(input string) (RenderChartData, error) {
	lines := strings.Split(strings.TrimSpace(input), "\n")

	var chart_type string
	var chart_height string
	var chart_label string
	var chart_title string
	var chart_color string

	var data_lines []string
	in_data := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "layout:"):
			chart_type = strings.TrimSpace(strings.TrimPrefix(line, "layout:"))
		case strings.HasPrefix(line, "height:"):
			chart_height = strings.TrimSpace(strings.TrimPrefix(line, "height:"))
		case strings.HasPrefix(line, "label:"):
			chart_label = strings.TrimSpace(strings.TrimPrefix(line, "label:"))
		case strings.HasPrefix(line, "title:"):
			chart_title = strings.TrimSpace(strings.TrimPrefix(line, "title:"))
		case strings.HasPrefix(line, "color:"):
			chart_color = strings.TrimSpace(strings.TrimPrefix(line, "color:"))
		case strings.HasPrefix(line, "data:"):
			in_data = true
			// add everything after `data:` in this line (if `[ ...` is on same line)
			if i := strings.Index(line, "["); i != -1 {
				data_lines = append(data_lines, line[i:])
			}
		default:
			if in_data {
				data_lines = append(data_lines, line)
			}
		}
	}

	if chart_type == "" {
		return RenderChartData{}, errors.New("layout not found")
	}
	if len(data_lines) == 0 {
		return RenderChartData{}, errors.New("data not found")
	}

	// Join and normalize to valid JSON
	data_str := strings.Join(data_lines, "\n")
	data_str = strings.TrimSuffix(data_str, "]")
	data_str = strings.TrimSpace(data_str)
	if !strings.HasPrefix(data_str, "[") {
		data_str = "[" + data_str
	}
	if !strings.HasSuffix(data_str, "]") {
		data_str = data_str + "]"
	}

	// Replace loose keys and quotes to JSON-compatible
	data_str = strings.ReplaceAll(data_str, "key:", `"key":`)
	data_str = strings.ReplaceAll(data_str, "value:", `"value":`)
	data_str = strings.ReplaceAll(data_str, "'", `"`)
	data_str = strings.ReplaceAll(data_str, ", }", "}")
	data_str = strings.ReplaceAll(data_str, ",]", "]")

	var points []RenderPoint
	if err := json.Unmarshal([]byte(data_str), &points); err != nil {
		return RenderChartData{}, fmt.Errorf("failed to parse chart data: %w", err)
	}

	keys_numeric := true
	for _, p := range points {
		key, ok := p.Key.(string)
		if ok {
			if _, err := fmt.Sscanf(key, "%v", new(float64)); err != nil {
				keys_numeric = false
				break
			}
		}
	}

	return RenderChartData{
		Type:        chart_type,
		Height:      chart_height,
		Label:       chart_label,
		Title:       chart_title,
		Points:      points,
		KeysNumeric: keys_numeric,
		Color:       chart_color,
	}, nil
}

func BuildChartJS(div_id string, cd RenderChartData) string {
	// Normalize type
	t := strings.ToLower(strings.TrimSpace(cd.Type))
	switch t {
	case "bar", "line", "pie":
	default:
		t = "bar"
	}

	// Prepare labels and values
	labels := make([]interface{}, len(cd.Points))
	values := make([]float64, len(cd.Points))
	for i, p := range cd.Points {
		labels[i] = p.Key
		values[i] = p.Value
	}
	labelsJSON, _ := json.Marshal(labels)
	valuesJSON, _ := json.Marshal(values)

	// UI color (text/grid). Default if not provided.
	uiColor := cd.Color
	if strings.TrimSpace(uiColor) == "" {
		uiColor = "#dddddd"
	}
	uiColorJSON, _ := json.Marshal(uiColor)

	// Title text: prefer Title; fallback to Label; if both empty, hide title.
	titleText := cd.Title
	if strings.TrimSpace(titleText) == "" {
		titleText = cd.Label
	}
	titleJSON, _ := json.Marshal(titleText)
	titleDisplay := "false"
	if strings.TrimSpace(titleText) != "" {
		titleDisplay = "true"
	}

	// Dataset (no explicit colors -> keep Chart.js defaults)
	dataset := fmt.Sprintf(`{
		label: %q,
		data: %s,
		borderWidth: 1
	}`, cd.Label, valuesJSON)

	// Options: style only the UI
	options := ""
	if t == "pie" {
		// Pie has no scales; just legend + title styling
		options = fmt.Sprintf(`{
			responsive: true,
			maintainAspectRatio: false,
			plugins: {
				legend: { labels: { color: %s } },
				title: { display: %s, text: %s, color: %s }
			}
		}`, uiColorJSON, titleDisplay, titleJSON, uiColorJSON)
	} else {
		// Bar/Line: add axes styling and subtle gridline color
		options = fmt.Sprintf(`{
			responsive: true,
			maintainAspectRatio: false,
			plugins: {
				legend: { labels: { color: %s } },
				title:  { display: %s, text: %s, color: %s }
			},
			scales: {
				x: {
					ticks: { color: %s },
					grid:  { color: "rgba(255,255,255,0.1)" }
				},
				y: {
					ticks: { color: %s },
					grid:  { color: "rgba(255,255,255,0.1)" }
				}
			}
		}`, uiColorJSON, titleDisplay, titleJSON, uiColorJSON, uiColorJSON, uiColorJSON)
	}

	return fmt.Sprintf(`
	<script defer>
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
	</script>`, div_id, labelsJSON, dataset, t, options)
}

// Finally render
func (r *HTMLRenderer) Render(w util.BufWriter, src []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ChartBlock)
	if !entering {
		// We do not do anything at the closing tag
		return ast.WalkContinue, nil
	}

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

	// Get chart data
	chart_data, err := ParseChartData(string(input_b))
	if err != nil {
		// Currently just ignore
		return ast.WalkContinue, err
	}

	out = append(out, []byte(fmt.Sprintf(`<div style="position:relative;width:100%%;height:%s"><canvas id="%s"></canvas></div>`, chart_data.Height, div_id))...)

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
