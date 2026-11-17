// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

// Package membudget provides a cumulative memory budget for parsing
// untrusted input.  A budget is sized at construction, decremented by
// each allocation, and discarded when the parse completes; the budget
// never grants memory back, so peak memory stays within the total it
// was sized for.
package membudget

import (
	"errors"
	"sync"
	"unsafe"
)

const (
	// PerAllocOverhead is the fixed cost which every charge adds to its
	// byte count, to account for slice-header and heap-block overhead.
	// This applies to [Budget.Charge], [Budget.ChargeN] and
	// [Budget.AllocSlice] alike.  Without this surcharge an attacker
	// can amplify input size by issuing many tiny allocations, each
	// costing far more than its payload.
	PerAllocOverhead = 32

	// MapEntryOverhead is the budget cost a caller should charge when
	// inserting a new entry into a Go map: the key, the slice or
	// pointer value header, and the map's bucket bookkeeping.  Callers
	// should not charge this for overwrites.
	MapEntryOverhead = 48
)

// ErrExceeded is returned when a charge would push the budget below zero.
// This includes a charge whose size is too large to be represented.
var ErrExceeded = errors.New("membudget: budget exceeded")

var errNegative = errors.New("membudget: negative argument")

// Budget tracks the remaining memory budget for a single parse.
// Budget is safe for concurrent use.
//
// A caller which wants no cap can use New(math.MaxInt64).
type Budget struct {
	mu        sync.Mutex
	remaining int64
}

// New returns a Budget which allows charges totalling at most remaining
// bytes, where each charge also costs [PerAllocOverhead].  remaining
// must be non-negative.
func New(remaining int64) *Budget {
	return &Budget{remaining: remaining}
}

// Charge subtracts bytes + [PerAllocOverhead] from the budget.  On
// exhaustion the budget is left unchanged and [ErrExceeded] is
// returned.  bytes must be non-negative.
func (b *Budget) Charge(bytes int) error {
	if bytes < 0 {
		return errNegative
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	// remaining >= 0, so the subtraction cannot wrap
	if b.remaining-PerAllocOverhead < int64(bytes) {
		return ErrExceeded
	}
	b.remaining -= int64(bytes) + PerAllocOverhead
	return nil
}

// ChargeN charges b for n elements of size bytes each, computing the total
// with overflow checking.  Use it when charging a homogeneous batch whose
// per-element size is computed at runtime rather than taken from a Go type;
// use [Budget.AllocSlice] when the element type is statically known.
//
// n and size must be non-negative.  A total too large to be represented
// returns [ErrExceeded].
func (b *Budget) ChargeN(n, size int) error {
	if n < 0 || size < 0 {
		return errNegative
	}
	bytes := n * size
	if size != 0 && bytes/size != n {
		return ErrExceeded
	}
	return b.Charge(bytes)
}

// Available returns the largest n for which a subsequent
// [Budget.Charge](n) would not return [ErrExceeded], or 0 if there is
// no such n.  The value is a hint only: a concurrent Charge may
// shrink the budget between an Available call and a subsequent Charge,
// so callers must still treat the eventual Charge as authoritative.
func (b *Budget) Available() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.remaining <= PerAllocOverhead {
		return 0
	}
	return b.remaining - PerAllocOverhead
}

// AllocSlice charges b for a slice of n elements of T and, if the
// charge succeeds, returns make([]T, n).
//
// For slice, map, interface, or string element types, only the header
// size is charged here; the referenced elements (slice backing array,
// map buckets, interface body, or string bytes) must be charged
// separately when allocated.
//
// n must be non-negative.  A byte count too large to be represented
// returns [ErrExceeded].
func (b *Budget) AllocSlice[T any](n int) ([]T, error) {
	var zero T
	if err := b.ChargeN(n, int(unsafe.Sizeof(zero))); err != nil {
		return nil, err
	}
	return make([]T, n), nil
}
