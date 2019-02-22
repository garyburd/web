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
	"reflect"
	"strings"
	"sync"
	ttemp "text/template"
)

type Template struct {
	fileNames       []string
	mimeType        string
	execute         func(w io.Writer, v interface{}) error
	executeTemplate func(w io.Writer, name string, v interface{}) error
	hasTemplate     func(name string) bool
	err             error // setup error
	once            sync.Once
	load            func()
}

func (t *Template) setup() error {
	t.once.Do(t.load)
	return t.err
}

func (t *Template) Execute(w io.Writer, v interface{}) error {
	if err := t.setup(); err != nil {
		return err
	}
	return t.execute(w, v)
}

func (t *Template) ExecuteTemplate(w io.Writer, name string, v interface{}) error {
	if err := t.setup(); err != nil {
		return err
	}
	return t.executeTemplate(w, name, v)
}

func (t *Template) HasTemplate(name string) bool {
	if err := t.setup(); err != nil {
		return false
	}
	return t.hasTemplate(name)
}

func (t *Template) WriteResponse(w http.ResponseWriter, r *http.Request, statusCode int, data interface{}) error {
	if err := t.setup(); err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := t.execute(&buf, data); err != nil {
		return err
	}
	w.Header().Set("Content-Type", t.mimeType)

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

type Manager struct {
	TextFuncs map[string]interface{}
	HTMLFuncs map[string]interface{}
	RootName  string // default is "ROOT"
	dir       string
	templates []*Template
	html      struct {
		mu    sync.Mutex
		base  *htemp.Template
		cache map[string]*htemp.Template
	}
	text struct {
		mu    sync.Mutex
		base  *ttemp.Template
		cache map[string]*ttemp.Template
	}
}

// NewHTML creates a new template with HTML escaping from the specified files.
// Order the files from most specific to this template to most common. The
// files in common suffixes across templates are parsed once. File names are
// relative to the directory name passed to Load.
func (m *Manager) NewHTML(fileNames ...string) *Template {
	t := &Template{fileNames: fileNames, mimeType: mime.TypeByExtension(path.Ext(fileNames[0]))}
	t.load = func() { m.loadHTML(t) }
	m.templates = append(m.templates, t)
	return t
}

// NewText creates a new template from the specified files. Order the files
// from most specific to this template to most common. The files in common
// suffixes across templates are parsed once. File names are relative to the
// directory name passed to Load.
func (m *Manager) NewText(fileNames ...string) *Template {
	t := &Template{fileNames: fileNames, mimeType: mime.TypeByExtension(path.Ext(fileNames[0]))}
	t.load = func() { m.loadText(t) }
	m.templates = append(m.templates, t)
	return t
}

var templatePtrType = reflect.TypeOf((*Template)(nil))
var tagfns = []struct {
	tag string
	fn  func(*Manager, ...string) *Template
}{{"html", (*Manager).NewHTML}, {"text", (*Manager).NewText}}

// NewFromFields creates templates for fields in the struct pointed to by sp
// with text or html field tags. The value of the tag is a space separated list
// of the template files.
func (m *Manager) NewFromFields(sp interface{}) {
	const mustBePointerToStruct = "NewFromFields argument must be a pointer to a struct"

	v := reflect.ValueOf(sp)
	if v.Kind() != reflect.Ptr {
		panic(mustBePointerToStruct)
	}
	v = v.Elem()
	if v.Kind() != reflect.Struct {
		panic(mustBePointerToStruct)
	}
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		ft := t.Field(i)
		if ft.Type != templatePtrType {
			continue
		}
		for _, tagfn := range tagfns {
			value := ft.Tag.Get(tagfn.tag)
			if value != "" {
				if ft.PkgPath != "" {
					panic(fmt.Sprintf("fields with %s tag must be exported", tagfn.tag))
				}
				f := v.Field(i)
				f.Set(reflect.ValueOf(tagfn.fn(m, strings.Fields(value)...)))
				break
			}
		}
	}
}

// Load loads the templates from the specified directory.
func (m *Manager) Load(dir string, preload bool) error {
	if m.RootName == "" {
		m.RootName = "ROOT"
	}
	m.dir = dir
	m.html.base = htemp.Must(htemp.New("_").Funcs(m.HTMLFuncs).Parse(`{{define "_"}}{{end}}`))
	m.html.cache = make(map[string]*htemp.Template)
	m.text.base = ttemp.Must(ttemp.New("_").Funcs(m.TextFuncs).Parse(`{{define "_"}}{{end}}`))
	m.text.cache = make(map[string]*ttemp.Template)

	if preload {
		for _, t := range m.templates {
			if err := t.setup(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *Manager) loadHTML(template *Template) {
	m.html.mu.Lock()
	defer m.html.mu.Unlock()

	t := m.html.base
	for i := len(template.fileNames) - 1; i >= 0; i-- {
		key := strings.Join(template.fileNames[i:], "\n")
		tt, ok := m.html.cache[key]
		if !ok {
			name := filepath.Join(m.dir, filepath.FromSlash(template.fileNames[i]))
			tt, template.err = t.Clone()
			if template.err != nil {
				return
			}
			tt, template.err = tt.ParseFiles(name)
			if template.err != nil {
				return
			}
			m.html.cache[key] = tt
		}
		t = tt
	}
	t = t.Lookup(m.RootName)
	if t == nil {
		template.err = fmt.Errorf("Could not find %q in %v", m.RootName, template.fileNames)
		return
	}
	template.execute = t.Execute
	template.executeTemplate = t.ExecuteTemplate
	lookup := t.Lookup
	template.hasTemplate = func(name string) bool { return lookup(name) != nil }
}

func (m *Manager) loadText(template *Template) {
	m.text.mu.Lock()
	defer m.text.mu.Unlock()

	t := m.text.base
	for i := len(template.fileNames) - 1; i >= 0; i-- {
		key := strings.Join(template.fileNames[i:], "\n")
		tt, ok := m.text.cache[key]
		if !ok {
			name := filepath.Join(m.dir, filepath.FromSlash(template.fileNames[i]))
			tt, template.err = t.Clone()
			if template.err != nil {
				return
			}
			tt, template.err = tt.ParseFiles(name)
			if template.err != nil {
				return
			}
			m.text.cache[key] = tt
		}
		t = tt
	}
	t = t.Lookup(m.RootName)
	if t == nil {
		template.err = fmt.Errorf("Could not find %q in %v", m.RootName, template.fileNames)
		return
	}
	template.execute = t.Execute
	template.executeTemplate = t.ExecuteTemplate
	lookup := t.Lookup
	template.hasTemplate = func(name string) bool { return lookup(name) != nil }
}
