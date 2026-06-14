// seehuhn.de/go/membudget - memory budgets for parsing untrusted input
// Copyright (C) 2026  Jochen Voss <voss@seehuhn.de>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

// Package membudget provides a cumulative memory budget for parsing
// untrusted input.  A budget is sized at construction, decremented by
// each allocation, and discarded when the parse completes; the budget
// never grants memory back, so it bounds peak memory at most by the
// total it was sized for.
//
// [Budget] is safe for concurrent use; charges from multiple
// goroutines are serialised internally.
package membudget

import (
	"errors"
	"sync"
	"unsafe"
)

const (
	// perAllocOverhead is the fixed cost added to every Charge, to
	// account for slice-header and heap-block overhead.  Without this
	// surcharge an attacker can amplify input size by issuing many
	// tiny allocations, each costing far more than its payload.
	perAllocOverhead = 32

	// MapEntryOverhead is the budget cost a caller should charge when
	// inserting a new entry into a Go map: the key, the slice or
	// pointer value header, and the map's bucket bookkeeping.  Callers
	// should not charge this for overwrites.
	MapEntryOverhead = 48
)

// ErrExceeded is returned by [Budget.Charge] when an allocation would
// push the budget below zero.
var ErrExceeded = errors.New("membudget: budget exceeded")

// ErrInvalid is returned when a requested size is not a sensible
// allocation size: a negative byte count or element count, or a size so
// large it cannot be represented.  It signals a caller bug rather than
// an exhausted budget, so it is distinct from [ErrExceeded].
var ErrInvalid = errors.New("membudget: invalid size")

// Budget tracks the remaining memory budget for a single parse.
// Budget is safe for concurrent use; charges from multiple goroutines
// are serialised internally.
//
// Callers that genuinely do not want a cap should construct a
// permissive budget with [New] and a generous limit; a nil *Budget is
// not supported.
type Budget struct {
	mu        sync.Mutex
	remaining int64
}

// New returns a Budget with the given byte budget.  Negative values
// are treated as zero.
func New(remaining int64) *Budget {
	if remaining < 0 {
		remaining = 0
	}
	return &Budget{remaining: remaining}
}

// Charge subtracts (bytes + perAllocOverhead) from the budget.  On
// exhaustion the budget is left unchanged and [ErrExceeded] is
// returned.  A negative bytes, or one large enough to overflow the
// internal accounting, returns [ErrInvalid].  Panics if b is nil.
func (b *Budget) Charge(bytes int) error {
	if bytes < 0 {
		return ErrInvalid
	}
	cost := int64(bytes) + perAllocOverhead
	if cost < int64(bytes) { // overflow guard
		return ErrInvalid
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.remaining < cost {
		return ErrExceeded
	}
	b.remaining -= cost
	return nil
}

// Available returns the largest n for which a subsequent
// [Budget.Charge](n) would not return [ErrExceeded].  The value is a
// hint only: a concurrent Charge may shrink the budget between an
// Available call and a subsequent Charge, so callers must still treat
// the eventual Charge as authoritative.  Panics if b is nil.
func (b *Budget) Available() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.remaining <= perAllocOverhead {
		return 0
	}
	return b.remaining - perAllocOverhead
}

// AllocSlice charges b for a slice of n elements of T and, if the
// charge succeeds, returns make([]T, n).  Panics if b is nil.
//
// For slice, map, interface, or string element types, only the header
// size is charged here; the referenced elements (slice backing array,
// map buckets, interface body, or string bytes) must be charged
// separately when allocated.
//
// A negative n, or one whose byte count overflows, returns [ErrInvalid].
func AllocSlice[T any](b *Budget, n int) ([]T, error) {
	var zero T
	size := int(unsafe.Sizeof(zero))
	if n < 0 {
		return nil, ErrInvalid
	}
	// overflow-safe multiplication for the byte count
	bytes := n * size
	if size != 0 && bytes/size != n {
		return nil, ErrInvalid
	}
	if err := b.Charge(bytes); err != nil {
		return nil, err
	}
	return make([]T, n), nil
}
