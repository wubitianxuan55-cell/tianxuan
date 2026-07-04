package builtin

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"tianxuan/internal/tool"
)

func init() { tool.RegisterBuiltin(codeIndex{}) }

type codeIndex struct{ workDir string }

func (codeIndex) Name() string { return "code_index" }

func (codeIndex) Description() string {
	return "轻量代码符号索引。outline 列出路径下所有符号；search 按名称/子串搜索符号定义。支持 Go AST 解析 + 多语言 regex 匹配。kind 可按 func/method/type/interface/struct/const/var 过滤。Prefer LSP tools (lsp_definition) for precise semantics."
}

func (codeIndex) Schema() json.RawMessage {
	return json.RawMessage(`{
"type":"object",
"properties":{
  "action":{"type":"string","enum":["outline","search"],"description":"outline lists symbols under path; search finds symbol definition candidates by name."},
  "path":{"type":"string","description":"File or directory path (default \".\")."},
  "query":{"type":"string","description":"Symbol name or substring for action=search."},
  "kind":{"type":"string","description":"Optional symbol kind filter: func, method, class, type, interface, const, var, struct, enum, trait."},
  "limit":{"type":"integer","description":"Maximum symbols to return (default 100, max 200).","minimum":1}
},
"required":["action"]
}`)
}

func (codeIndex) ReadOnly() bool { return true }

const (
	codeIdxDefaultLimit = 100
	codeIdxMaxLimit     = 200
	codeIdxMaxFileSize  = 1 << 20
	codeIdxMaxFiles     = 2000
)

type codeSymbol struct {
	Name      string
	Kind      string
	File      string
	Line      int
	Parent    string
	Signature string
}

func (c codeIndex) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Action string `json:"action"`
		Path   string `json:"path"`
		Query  string `json:"query"`
		Kind   string `json:"kind"`
		Limit  int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	p.Action = strings.ToLower(strings.TrimSpace(p.Action))
	if p.Action != "outline" && p.Action != "search" {
		return "", fmt.Errorf("action must be outline or search")
	}
	if p.Path == "" {
		p.Path = "."
	}
	if p.Limit <= 0 {
		p.Limit = codeIdxDefaultLimit
	}
	if p.Limit > codeIdxMaxLimit {
		p.Limit = codeIdxMaxLimit
	}

	p.Path = resolveIn(c.workDir, p.Path)
	kindFilter := strings.ToLower(strings.TrimSpace(p.Kind))

	files, err := c.collectFiles(p.Path)
	if err != nil {
		return "", err
	}

	var symbols []codeSymbol
	for _, f := range files {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		syms := c.parseFile(f)
		for _, s := range syms {
			if kindFilter != "" && s.Kind != kindFilter {
				continue
			}
			if p.Action == "search" && p.Query != "" && !strings.Contains(strings.ToLower(s.Name), strings.ToLower(p.Query)) {
				continue
			}
			symbols = append(symbols, s)
		}
	}

	sort.Slice(symbols, func(i, j int) bool {
		if symbols[i].File != symbols[j].File {
			return symbols[i].File < symbols[j].File
		}
		return symbols[i].Line < symbols[j].Line
	})

	if len(symbols) > p.Limit {
		symbols = symbols[:p.Limit]
	}

	if len(symbols) == 0 {
		return "(no matching symbols)", nil
	}

	var b strings.Builder
	for _, s := range symbols {
		line := fmt.Sprintf("%s:%d: %s %s", s.File, s.Line, s.Kind, s.Name)
		if s.Parent != "" {
			line += fmt.Sprintf(" (receiver: %s)", s.Parent)
		}
		if s.Signature != "" {
			line += fmt.Sprintf(" %s", s.Signature)
		}
		b.WriteString(line + "\n")
	}
	return strings.TrimSpace(b.String()), nil
}

func (c codeIndex) collectFiles(root string) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", root, err)
	}
	if !info.IsDir() {
		return []string{root}, nil
	}
	var files []string
	walkErr := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Size() > codeIdxMaxFileSize {
			return nil
		}
		if len(files) >= codeIdxMaxFiles {
			return filepath.SkipAll
		}
		files = append(files, path)
		return nil
	})
	if walkErr != nil && walkErr != filepath.SkipAll {
		return nil, walkErr
	}
	return files, nil
}

func (c codeIndex) parseFile(path string) []codeSymbol {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return c.parseGoFile(path)
	default:
		return c.parseWithRegex(path, ext)
	}
}

// ── Go AST parser ─────────────────────────────────────────────────────

func (c codeIndex) parseGoFile(path string) []codeSymbol {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil
	}
	var symbols []codeSymbol
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			kind := "func"
			if d.Recv != nil && len(d.Recv.List) > 0 {
				kind = "method"
			}
			parent := ""
			if d.Recv != nil && len(d.Recv.List) > 0 {
				parent = receiverName(d.Recv.List[0].Type)
			}
			symbols = append(symbols, codeSymbol{
				Name:   d.Name.Name,
				Kind:   kind,
				File:   shortPath(c.workDir, path),
				Line:   fset.Position(d.Pos()).Line,
				Parent: parent,
			})
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					kind := "type"
					if _, isStruct := s.Type.(*ast.StructType); isStruct {
						kind = "struct"
					} else if _, isInterface := s.Type.(*ast.InterfaceType); isInterface {
						kind = "interface"
					}
					symbols = append(symbols, codeSymbol{
						Name: s.Name.Name,
						Kind: kind,
						File: shortPath(c.workDir, path),
						Line: fset.Position(s.Pos()).Line,
					})
				case *ast.ValueSpec:
					for _, name := range s.Names {
						if name.Name == "_" {
							continue
						}
						kind := "var"
						if d.Tok == token.CONST {
							kind = "const"
						}
						symbols = append(symbols, codeSymbol{
							Name: name.Name,
							Kind: kind,
							File: shortPath(c.workDir, path),
							Line: fset.Position(name.Pos()).Line,
						})
					}
				}
			}
		}
	}
	return symbols
}

func receiverName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		if id, ok := e.X.(*ast.Ident); ok {
			return "*" + id.Name
		}
	}
	return ""
}

// ── Regex matchers for non-Go languages ────────────────────────────────

var regexMatchers = map[string][]struct {
	kind  string
	regex *regexp.Regexp
}{
	".py": {
		{"func", regexp.MustCompile(`^\s*def\s+(\w+)`)},
		{"class", regexp.MustCompile(`^\s*class\s+(\w+)`)},
	},
	".js": {
		{"func", regexp.MustCompile(`(?:function\s+(\w+)|(\w+)\s*=\s*(?:async\s+)?function|(\w+)\s*=\s*\([^)]*\)\s*=>|(\w+)\s*\([^)]*\)\s*\{)`)},

		{"class", regexp.MustCompile(`class\s+(\w+)`)},
	},
	".ts": {
		{"func", regexp.MustCompile(`(?:function\s+(\w+)|(\w+)\s*=\s*(?:async\s+)?function|(\w+)\s*=\s*\([^)]*\)\s*=>|(\w+)\s*\([^)]*\)\s*\{)`)},

		{"class", regexp.MustCompile(`class\s+(\w+)`)},
		{"interface", regexp.MustCompile(`interface\s+(\w+)`)},
	},
	".java": {
		{"class", regexp.MustCompile(`class\s+(\w+)`)},
		{"interface", regexp.MustCompile(`interface\s+(\w+)`)},
		{"func", regexp.MustCompile(`(?:public|private|protected|static|\s)+[\w<>[\],\s]+\s+(\w+)\s*\(`)},
	},
	".rs": {
		{"func", regexp.MustCompile(`fn\s+(\w+)`)},
		{"struct", regexp.MustCompile(`struct\s+(\w+)`)},
		{"trait", regexp.MustCompile(`trait\s+(\w+)`)},
		{"enum", regexp.MustCompile(`enum\s+(\w+)`)},
	},
	".c": {
		{"func", regexp.MustCompile(`(?:static\s+)?[\w*]+\s+(\w+)\s*\(`)},
	},
	".cpp": {
		{"func", regexp.MustCompile(`(?:static\s+)?[\w:*<>]+\s+(\w+)\s*\(`)},
		{"class", regexp.MustCompile(`class\s+(\w+)`)},
	},
}

func (c codeIndex) parseWithRegex(path, ext string) []codeSymbol {
	matchers, ok := regexMatchers[ext]
	if !ok {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var symbols []codeSymbol
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := sc.Text()
		for _, m := range matchers {
			matches := m.regex.FindStringSubmatch(line)
			if matches == nil {
				continue
			}
			name := ""
			for i := 1; i < len(matches); i++ {
				if matches[i] != "" {
					name = matches[i]
					break
				}
			}
			if name != "" && name != "_" {
				symbols = append(symbols, codeSymbol{
					Name: name,
					Kind: m.kind,
					File: shortPath(c.workDir, path),
					Line: lineNo,
				})
			}
		}
	}
	return symbols
}

func shortPath(workDir, path string) string {
	if workDir != "" {
		if rel, err := filepath.Rel(workDir, path); err == nil && !strings.HasPrefix(rel, "..") {
			return rel
		}
	}
	return path
}
