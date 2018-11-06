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
	TextFuncs     map[string]interface{}
	HTMLFuncs     map[string]interface{}
	RootName      string // default is "ROOT"
	htmlTemplates []*Template
	textTemplates []*Template
}

// NewHTML creates a new template with HTML escaping from the specified files.
// Order the files from most specific to this template to most common. The
// files in common suffixes across templates are parsed once. File names are
// relative to the directory name passed to Load.
func (m *TemplateManager) NewHTML(fileNames ...string) *Template {
	t := &Template{fileNames: fileNames, contentType: mime.TypeByExtension(path.Ext(fileNames[0]))}
	m.htmlTemplates = append(m.htmlTemplates, t)
	return t
}

// NewText creates a new template from the specified files. Order the files
// from most specific to this template to most common. The files in common
// suffixes across templates are parsed once. File names are relative to the
// directory name passed to Load.
func (m *TemplateManager) NewText(fileNames ...string) *Template {
	t := &Template{fileNames: fileNames, contentType: mime.TypeByExtension(path.Ext(fileNames[0]))}
	m.textTemplates = append(m.textTemplates, t)
	return t
}

// Load loads the templates from the specified directory.
func (m *TemplateManager) Load(dir string) error {
	if m.RootName == "" {
		m.RootName = "ROOT"
	}
	if err := m.loadHTML(dir); err != nil {
		return err
	}
	return m.loadText(dir)
}

func (m *TemplateManager) loadHTML(dir string) error {

	base := htemp.Must(htemp.New("_").Funcs(m.HTMLFuncs).Parse(`{{define "_"}}{{end}}`))
	cache := make(map[string]*htemp.Template)

	for _, template := range m.htmlTemplates {
		t := base
		for i := len(template.fileNames) - 1; i >= 0; i-- {
			key := strings.Join(template.fileNames[i:], "\n")
			tt, ok := cache[key]
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
				cache[key] = tt
			}
			t = tt
		}
		t = t.Lookup(m.RootName)
		if t == nil {
			return fmt.Errorf("Could not find %q in %v", m.RootName, template.fileNames)
		}
		template.execute = t.Execute
		template.executeTemplate = t.ExecuteTemplate
		lookup := t.Lookup
		template.hasTemplate = func(name string) bool { return lookup(name) != nil }
	}
	return nil
}

func (m *TemplateManager) loadText(dir string) error {

	base := ttemp.Must(ttemp.New("_").Funcs(m.TextFuncs).Parse(`{{define "_"}}{{end}}`))
	cache := make(map[string]*ttemp.Template)

	for _, template := range m.textTemplates {
		t := base
		for i := len(template.fileNames) - 1; i >= 0; i-- {
			key := strings.Join(template.fileNames[i:], "\n")
			tt, ok := cache[key]
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
				cache[key] = tt
			}
			t = tt
		}
		t = t.Lookup(m.RootName)
		if t == nil {
			return fmt.Errorf("Could not find %q in %v", m.RootName, template.fileNames)
		}
		template.execute = t.Execute
		template.executeTemplate = t.ExecuteTemplate
		lookup := t.Lookup
		template.hasTemplate = func(name string) bool { return lookup(name) != nil }
	}
	return nil
}
