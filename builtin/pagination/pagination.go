// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pagination provides zero-allocation API request pagination, limit bounding,
// offset calculation, sorting extraction, and response metadata builder utilities.
package pagination

import (
	"math"
	"strconv"
	"strings"

	"github.com/lemon4ksan/sein"
)

// Params contains extracted and validated pagination parameters.
type Params struct {
	// Page is the 1-based page number.
	Page int `json:"page"`
	// Limit is the page size limit.
	Limit int `json:"limit"`
	// Offset is the computed SQL offset ((Page - 1) * Limit).
	Offset int `json:"offset"`
	// Sort is the requested sorting field.
	Sort string `json:"sort,omitempty"`
	// Order is the sort direction ("asc" or "desc").
	Order string `json:"order,omitempty"`
}

// Metadata contains calculated pagination response metadata.
type Metadata struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	TotalItems int64 `json:"total_items"`
	TotalPages int   `json:"total_pages"`
	HasNext    bool  `json:"has_next"`
	HasPrev    bool  `json:"has_prev"`
}

// Paginated wraps a slice of items with pagination metadata.
type Paginated[T any] struct {
	Items []T      `json:"items"`
	Meta  Metadata `json:"meta"`
}

// BuildMeta calculates pagination metadata from total item count and query params.
func BuildMeta(totalItems int64, p Params) Metadata {
	limit := p.Limit
	if limit <= 0 {
		limit = 20
	}

	page := p.Page
	if page <= 0 {
		page = 1
	}

	totalPages := int(math.Ceil(float64(totalItems) / float64(limit)))
	if totalPages < 1 && totalItems == 0 {
		totalPages = 0
	}

	return Metadata{
		Page:       page,
		Limit:      limit,
		TotalItems: totalItems,
		TotalPages: totalPages,
		HasNext:    page < totalPages,
		HasPrev:    page > 1 && totalPages > 0,
	}
}

// NewPaginated creates a Paginated container wrapping items with metadata.
func NewPaginated[T any](items []T, totalItems int64, p Params) Paginated[T] {
	return Paginated[T]{
		Items: items,
		Meta:  BuildMeta(totalItems, p),
	}
}

// Config configures the Pagination middleware.
type Config struct {
	// DefaultPage is the fallback page when unspecified. Default is 1.
	DefaultPage int
	// DefaultLimit is the fallback limit when unspecified. Default is 20.
	DefaultLimit int
	// MaxLimit is the maximum permissible limit. Default is 100.
	MaxLimit int
	// PageParam is the page query parameter name. Default is "page".
	PageParam string
	// LimitParam is the limit query parameter name. Default is "limit".
	LimitParam string
	// SortParam is the sort query parameter name. Default is "sort".
	SortParam string
	// OrderParam is the order query parameter name. Default is "order".
	OrderParam string
}

// Option configures Pagination settings.
type Option func(*Config)

// WithDefaultLimit sets the default page limit.
func WithDefaultLimit(limit int) Option {
	return func(c *Config) {
		c.DefaultLimit = limit
	}
}

// WithMaxLimit sets the maximum permissible page limit.
func WithMaxLimit(max int) Option {
	return func(c *Config) {
		c.MaxLimit = max
	}
}

// From extracts pagination parameters from the request context.
func From(req *sein.Request) Params {
	if p, ok := sein.Get[Params](req); ok {
		return p
	}

	return Params{Page: 1, Limit: 20, Offset: 0}
}

// New creates a pagination extraction and bounding middleware.
func New(opts ...Option) sein.Middleware {
	cfg := Config{
		DefaultPage:  1,
		DefaultLimit: 20,
		MaxLimit:     100,
		PageParam:    "page",
		LimitParam:   "limit",
		SortParam:    "sort",
		OrderParam:   "order",
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	return func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			page := cfg.DefaultPage
			if pStr := string(req.Query(cfg.PageParam)); pStr != "" {
				if val, err := strconv.Atoi(pStr); err == nil && val > 0 {
					page = val
				}
			}

			limit := cfg.DefaultLimit
			if lStr := string(req.Query(cfg.LimitParam)); lStr != "" {
				if val, err := strconv.Atoi(lStr); err == nil && val > 0 {
					limit = val
				}
			}

			if cfg.MaxLimit > 0 && limit > cfg.MaxLimit {
				limit = cfg.MaxLimit
			}

			offset := (page - 1) * limit
			sortField := strings.TrimSpace(string(req.Query(cfg.SortParam)))
			order := strings.ToLower(strings.TrimSpace(string(req.Query(cfg.OrderParam))))
			if order != "desc" {
				order = "asc"
			}

			params := Params{
				Page:   page,
				Limit:  limit,
				Offset: offset,
				Sort:   sortField,
				Order:  order,
			}

			sein.Set(req, params)

			return next(req)
		}
	}
}
