{{.Header}}

package {{.Package}}

import "gorm.io/gorm"

// ListOptions controls pagination, ordering, and filtering for the generated
// List and Count store methods. Where accepts anything gorm.DB.Where understands
// (a struct, a map, or an SQL fragment with Args), e.g.
//
//	ListOptions{Where: "published_year = ?", Args: []any{2024}, OrderBy: "title", Limit: 20}
type ListOptions struct {
	Limit   int
	Offset  int
	OrderBy string
	Where   any
	Args    []any
}

// apply layers the options onto a query. Count callers apply only Where (see the
// generated Count methods), since limit/offset must not change the total.
func (o ListOptions) apply(db *gorm.DB) *gorm.DB {
	if o.Where != nil {
		db = db.Where(o.Where, o.Args...)
	}
	if o.OrderBy != "" {
		db = db.Order(o.OrderBy)
	}
	if o.Limit > 0 {
		db = db.Limit(o.Limit)
	}
	if o.Offset > 0 {
		db = db.Offset(o.Offset)
	}
	return db
}
