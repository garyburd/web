package templates

import (
	"bytes"
	"compress/gzip"
	"fmt"
	htemp "html/template"
	"io"
	"mime"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	ttemp "text/template"
)

type Template struct {
	fileNames       []string
	contentType     string
	execute         func(w io.Writer, v interface{}) error
	executeTemplate func(w io.Writer, name string, v interface{}) error
	hasTemplate     func(name string) bool
}

func (t *Template) Execute(w io.Writer, v interface{}) error {
	return t.execute(w, v)
}

func (t *Template) ExecuteTemplate(w io.Writer, name string, v interface{}) error {
	return t.executeTemplate(w, name, v)
}

func (t *Template) HasTemplate(name string) bool {
	return t.hasTemplate(name)
}

func (t *Template) WriteResponse(w http.ResponseWriter, r *http.Request, statusCode int, data interface{}) error {
	var buf bytes.Buffer
	if err := t.execute(&buf, data); err != nil {
		return err
	}
	w.Header().Set("Content-Type", t.contentType)

	if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.WriteHeader(statusCode)
		w.Write(buf.Bytes())
		return nil
	}

	w.Header().Set("Content-Encoding", "gzip")
	w.WriteHeader(statusCode)
	gzw := gzip.NewWriter(w)
	gzw.Write(buf.Bytes())
	gzw.Close()
	return nil
}

type TemplateManager struct {
	TextFuncs map[string]interface{}
	HTMLFuncs map[string]interface{}
	RootName  string // default is "ROOT"
	templates []*Template
}

func (m *TemplateManager) New(fileNames ...string) *Template {
	t := &Template{fileNames: fileNames, contentType: mime.TypeByExtension(path.Ext(fileNames[0]))}
	m.templates = append(m.templates, t)
	return t
}

func (m *TemplateManager) Load(dir string) error {
	hbase := htemp.Must(htemp.New("_").Funcs(m.HTMLFuncs).Parse(`{{define "_"}}{{end}}`))
	hcache := make(map[string]*htemp.Template)
	tbase := ttemp.Must(ttemp.New("_").Funcs(m.TextFuncs).Parse(`{{define "_"}}{{end}}`))
	tcache := make(map[string]*ttemp.Template)

	root := m.RootName
	if root == "" {
		root = "ROOT"
	}

	for _, template := range m.templates {
		if strings.HasSuffix(template.fileNames[0], ".html") {
			t := hbase
			for i := len(template.fileNames) - 1; i >= 0; i-- {
				key := strings.Join(template.fileNames[i:], "\n")
				tt, ok := hcache[key]
				if !ok {
					name := filepath.Join(dir, filepath.FromSlash(template.fileNames[i]))
					var err error
					tt, err = t.Clone()
					if err != nil {
						return err
					}
					tt, err = tt.ParseFiles(name)
					if err != nil {
						return err
					}
					hcache[key] = tt
				}
				t = tt
			}
			t = t.Lookup(root)
			if t == nil {
				return fmt.Errorf("Could not find %q in %v", root, template.fileNames)
			}
			template.execute = t.Execute
			template.executeTemplate = t.ExecuteTemplate
			lookup := t.Lookup
			template.hasTemplate = func(name string) bool { return lookup(name) != nil }
		} else {
			t := tbase
			for i := len(template.fileNames) - 1; i >= 0; i-- {
				key := strings.Join(template.fileNames[i:], "\n")
				tt, ok := tcache[key]
				if !ok {
					name := filepath.Join(dir, filepath.FromSlash(template.fileNames[i]))
					var err error
					tt, err = t.Clone()
					if err != nil {
						return err
					}
					tt, err = tt.ParseFiles(name)
					if err != nil {
						return err
					}
					tcache[key] = tt
				}
				t = tt
			}
			t = t.Lookup(root)
			if t == nil {
				return fmt.Errorf("Could not find %q in %v", root, template.fileNames)
			}
			template.execute = t.Execute
			template.executeTemplate = t.ExecuteTemplate
			lookup := t.Lookup
			template.hasTemplate = func(name string) bool { return lookup(name) != nil }
		}
	}
	return nil
}
