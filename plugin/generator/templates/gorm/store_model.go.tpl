{{.Header}}

package {{.Package}}

{{.Imports}}

// {{.Store}} provides typed CRUD access to {{.Name}} records.
// {{.Comment}}
type {{.Store}} struct{ DB *gorm.DB }
{{- if .AssertStore}}

// Compile-time proof that {{.Store}} satisfies the generic gormx.Store, so the
// generic engine can drive it alongside the typed finders below.
var _ gormx.Store[{{.Name}}] = (*{{.Store}})(nil)
{{- end}}

// New{{.Store}} returns a {{.Store}} backed by db.
func New{{.Store}}(db *gorm.DB) *{{.Store}} { return &{{.Store}}{DB: db} }

// Create inserts m.
func (s *{{.Store}}) Create(ctx context.Context, m *{{.Name}}) error {
	return s.DB.WithContext(ctx).Create(m).Error
}

// List returns the {{.Name}} records matching opts.
func (s *{{.Store}}) List(ctx context.Context, opts gormx.ListOptions) ([]{{.Name}}, error) {
	var out []{{.Name}}
	if err := opts.Apply(s.DB.WithContext(ctx)).Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// Count returns the number of {{.Name}} records matching opts.Where
// (pagination and ordering are ignored).
func (s *{{.Store}}) Count(ctx context.Context, opts gormx.ListOptions) (int64, error) {
	db := s.DB.WithContext(ctx).Model(&{{.Name}}{})
	if opts.Where != nil {
		db = db.Where(opts.Where, opts.Args...)
	}
	var n int64
	if err := db.Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

// Update persists every field of m, which must carry its primary key.
func (s *{{.Store}}) Update(ctx context.Context, m *{{.Name}}) error {
	return s.DB.WithContext(ctx).Save(m).Error
}
{{if .HasPK}}
// GetByID fetches the {{.Name}} with the given primary key.
func (s *{{.Store}}) GetByID(ctx context.Context, id {{.PKArgType}}) (*{{.Name}}, error) {
	var m {{.Name}}
	if err := s.DB.WithContext(ctx).First(&m, "{{.PKColumn}} = ?", id).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

// DeleteByID removes the {{.Name}} with the given primary key.
func (s *{{.Store}}) DeleteByID(ctx context.Context, id {{.PKArgType}}) error {
	return s.DB.WithContext(ctx).Delete(&{{.Name}}{}, "{{.PKColumn}} = ?", id).Error
}
{{end}}
{{- range .UniqueFinders}}
// {{.Method}} fetches the {{$.Name}} with the given {{.Column}} (a unique column).
func (s *{{$.Store}}) {{.Method}}(ctx context.Context, v {{.ArgType}}) (*{{$.Name}}, error) {
	var m {{$.Name}}
	if err := s.DB.WithContext(ctx).First(&m, "{{.Column}} = ?", v).Error; err != nil {
		return nil, err
	}
	return &m, nil
}
{{end}}
{{- range .FKFinders}}
// {{.Method}} returns the {{$.Name}} records whose {{.Column}} matches id, with opts applied.
func (s *{{$.Store}}) {{.Method}}(ctx context.Context, id {{.ArgType}}, opts gormx.ListOptions) ([]{{$.Name}}, error) {
	var out []{{$.Name}}
	q := opts.Apply(s.DB.WithContext(ctx).Where("{{.Column}} = ?", id))
	if err := q.Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}
{{end}}
