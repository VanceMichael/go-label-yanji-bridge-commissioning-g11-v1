package repository

import "time"

type Page struct {
	Limit  int
	Offset int
	Sort   string
	Status string
}

func (p Page) Normalize() Page {
	if p.Limit <= 0 {
		p.Limit = 25
	}
	if p.Limit > 100 {
		p.Limit = 100
	}
	if p.Offset < 0 {
		p.Offset = 0
	}
	if p.Sort != "target_open_at" {
		p.Sort = "created_at"
	}
	return p
}

type IdempotencyScope struct {
	Organization string
	Method       string
	Path         string
	Key          string
}

type IdempotencyRecord struct {
	Scope       IdempotencyScope
	RequestHash string
	StatusCode  int
	Response    []byte
	CreatedAt   time.Time
	ExpiresAt   time.Time
}
