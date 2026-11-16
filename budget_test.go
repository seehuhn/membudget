// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package membudget

import (
	"errors"
	"math"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"unsafe"
)

func TestNew(t *testing.T) {
	t.Run("zero", func(t *testing.T) {
		if err := New(0).Charge(0); !errors.Is(err, ErrExceeded) {
			t.Fatalf("err = %v, want ErrExceeded", err)
		}
	})

	t.Run("exact fit", func(t *testing.T) {
		b := New(1000)
		if err := b.Charge(1000 - PerAllocOverhead); err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		if err := b.Charge(0); !errors.Is(err, ErrExceeded) {
			t.Fatalf("err = %v, want ErrExceeded", err)
		}
	})

	t.Run("one byte over", func(t *testing.T) {
		b := New(1000)
		if err := b.Charge(1000 - PerAllocOverhead + 1); !errors.Is(err, ErrExceeded) {
			t.Fatalf("err = %v, want ErrExceeded", err)
		}
	})
}

func TestCharge(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		b := New(1000)
		before := b.Available()
		if err := b.Charge(100); err != nil {
			t.Fatal(err)
		}
		if got, want := before-b.Available(), int64(100+PerAllocOverhead); got != want {
			t.Errorf("consumed = %d, want %d", got, want)
		}
	})

	t.Run("exhaustion leaves budget unchanged", func(t *testing.T) {
		b := New(1000)
		before := b.Available()
		if err := b.Charge(1000); !errors.Is(err, ErrExceeded) {
			t.Fatalf("err = %v, want ErrExceeded", err)
		}
		if got := b.Available(); got != before {
			t.Errorf("Available = %d, want %d", got, before)
		}
	})

	t.Run("overflow errors", func(t *testing.T) {
		// bytes + PerAllocOverhead can only wrap around with a 64-bit int
		if strconv.IntSize < 64 {
			t.Skip("int is narrower than 64 bits")
		}
		if err := New(math.MaxInt64).Charge(math.MaxInt - PerAllocOverhead); err != nil {
			t.Fatalf("Charge(MaxInt - PerAllocOverhead): %v", err)
		}
		if err := New(math.MaxInt64).Charge(math.MaxInt); !errors.Is(err, ErrExceeded) {
			t.Fatalf("err = %v, want ErrExceeded", err)
		}
	})
}

func TestAvailable(t *testing.T) {
	t.Run("predicts next charge", func(t *testing.T) {
		b := New(1000)
		// Available reports the largest n that Charge(n) accepts, so
		// the PerAllocOverhead surcharge is already deducted.
		if got, want := b.Available(), int64(1000-PerAllocOverhead); got != want {
			t.Errorf("Available = %d, want %d", got, want)
		}
		// charging exactly Available must succeed and leave Available at zero
		if err := b.Charge(int(b.Available())); err != nil {
			t.Fatalf("Charge(Available): %v", err)
		}
		if got := b.Available(); got != 0 {
			t.Errorf("Available after draining = %d, want 0", got)
		}
	})

	t.Run("clamps at zero", func(t *testing.T) {
		// remaining below the surcharge can never satisfy any Charge
		b := New(PerAllocOverhead - 1)
		if got := b.Available(); got != 0 {
			t.Errorf("Available = %d, want 0", got)
		}
	})
}

func TestSurcharge(t *testing.T) {
	// many tiny charges drain the budget faster than the sum of the
	// payloads, by PerAllocOverhead per call.
	b := New(10 * PerAllocOverhead)
	for i := range 10 {
		if err := b.Charge(0); err != nil {
			t.Fatalf("charge %d: %v", i, err)
		}
	}
	if err := b.Charge(0); !errors.Is(err, ErrExceeded) {
		t.Fatalf("11th charge: err = %v, want ErrExceeded", err)
	}
}

// TestConcurrentCharge exercises Charge from many goroutines and
// verifies that exactly as many charges succeed as the budget allows
// for, with no over-debit and no race-detector report.
func TestConcurrentCharge(t *testing.T) {
	const (
		goroutines = 32
		perCall    = 8
		// each Charge costs perCall + PerAllocOverhead bytes
	)
	costPerCall := int64(perCall + PerAllocOverhead)
	const want = 100 // expected successful charges
	b := New(costPerCall * want)

	var success atomic.Int64
	var wg sync.WaitGroup
	for range goroutines {
		wg.Go(func() {
			for {
				if err := b.Charge(perCall); err != nil {
					return
				}
				success.Add(1)
			}
		})
	}
	wg.Wait()

	if got := success.Load(); got != want {
		t.Errorf("successful charges = %d, want %d", got, want)
	}
	if got := b.Available(); got != 0 {
		t.Errorf("Available = %d, want 0", got)
	}
}

func TestChargeN(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		b := New(1000)
		before := b.Available()
		if err := b.ChargeN(10, 8); err != nil {
			t.Fatal(err)
		}
		if got, want := before-b.Available(), int64(10*8+PerAllocOverhead); got != want {
			t.Errorf("consumed = %d, want %d", got, want)
		}
	})

	t.Run("zero size", func(t *testing.T) {
		b := New(1000)
		if err := b.ChargeN(1<<30, 0); err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
	})

	t.Run("exhaustion", func(t *testing.T) {
		b := New(1000)
		before := b.Available()
		if err := b.ChargeN(1000, 8); !errors.Is(err, ErrExceeded) {
			t.Fatalf("err = %v, want ErrExceeded", err)
		}
		if got := b.Available(); got != before {
			t.Errorf("Available = %d, want %d", got, before)
		}
	})

	t.Run("multiplication overflow", func(t *testing.T) {
		b := New(1 << 40)
		if err := b.ChargeN(int(^uint(0)>>1), 8); !errors.Is(err, ErrExceeded) {
			t.Fatalf("err = %v, want ErrExceeded", err)
		}
	})
}

func TestAllocSlice(t *testing.T) {
	t.Run("uint16", func(t *testing.T) {
		b := New(1000)
		before := b.Available()
		s, err := b.AllocSlice[uint16](100)
		if err != nil {
			t.Fatal(err)
		}
		if len(s) != 100 {
			t.Errorf("len = %d, want 100", len(s))
		}
		if got, want := before-b.Available(), int64(100*2+PerAllocOverhead); got != want {
			t.Errorf("consumed = %d, want %d", got, want)
		}
	})

	t.Run("pointer", func(t *testing.T) {
		b := New(1000)
		before := b.Available()
		s, err := b.AllocSlice[*int](4)
		if err != nil {
			t.Fatal(err)
		}
		if len(s) != 4 {
			t.Errorf("len = %d, want 4", len(s))
		}
		if got, want := before-b.Available(), int64(4*unsafe.Sizeof(uintptr(0))+PerAllocOverhead); got != want {
			t.Errorf("consumed = %d, want %d", got, want)
		}
	})

	t.Run("exhaustion", func(t *testing.T) {
		b := New(1000)
		before := b.Available()
		s, err := b.AllocSlice[uint16](1000)
		if !errors.Is(err, ErrExceeded) {
			t.Fatalf("err = %v, want ErrExceeded", err)
		}
		if s != nil {
			t.Errorf("got slice %v, want nil", s)
		}
		if got := b.Available(); got != before {
			t.Errorf("Available = %d, want %d", got, before)
		}
	})

	t.Run("multiplication overflow", func(t *testing.T) {
		// pick an n where n * sizeof(uint16) overflows
		s, err := New(1 << 40).AllocSlice[uint16](int(^uint(0) >> 1))
		if !errors.Is(err, ErrExceeded) {
			t.Fatalf("err = %v, want ErrExceeded", err)
		}
		if s != nil {
			t.Errorf("got slice %v, want nil", s)
		}
	})
}
