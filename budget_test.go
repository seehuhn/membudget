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

package membudget

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestNew(t *testing.T) {
	cases := []struct {
		name string
		in   int64
		want int64
	}{
		{"zero", 0, 0},
		{"positive", 1000, 1000},
		{"negative clamped", -1, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := New(c.in).remaining; got != c.want {
				t.Errorf("remaining = %d, want %d", got, c.want)
			}
		})
	}
}

func TestCharge(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		b := New(1000)
		if err := b.Charge(100); err != nil {
			t.Fatal(err)
		}
		if got, want := b.remaining, int64(1000-100-perAllocOverhead); got != want {
			t.Errorf("remaining = %d, want %d", got, want)
		}
	})

	t.Run("exhaustion leaves budget unchanged", func(t *testing.T) {
		b := New(10)
		if err := b.Charge(100); !errors.Is(err, ErrExceeded) {
			t.Fatalf("err = %v, want ErrExceeded", err)
		}
		if b.remaining != 10 {
			t.Errorf("remaining = %d, want 10", b.remaining)
		}
	})

	t.Run("negative size errors", func(t *testing.T) {
		b := New(1000)
		if err := b.Charge(-1); !errors.Is(err, ErrExceeded) {
			t.Fatalf("err = %v, want ErrExceeded", err)
		}
	})

	t.Run("overflow errors", func(t *testing.T) {
		// bytes + perAllocOverhead must not wrap around int64
		b := New(1 << 62)
		const maxInt = int(^uint(0) >> 1)
		if err := b.Charge(maxInt); !errors.Is(err, ErrExceeded) {
			t.Fatalf("err = %v, want ErrExceeded", err)
		}
	})

	t.Run("nil receiver is no-op", func(t *testing.T) {
		var b *Budget
		if err := b.Charge(1 << 30); err != nil {
			t.Fatal(err)
		}
	})
}

func TestSurcharge(t *testing.T) {
	// many tiny charges drain the budget faster than the sum of the
	// payloads, by perAllocOverhead per call.
	b := New(10 * perAllocOverhead)
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
		// each Charge costs perCall + perAllocOverhead bytes
	)
	costPerCall := int64(perCall + perAllocOverhead)
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
	if b.remaining != 0 {
		t.Errorf("remaining = %d, want 0", b.remaining)
	}
}

func TestAllocSlice(t *testing.T) {
	t.Run("uint16", func(t *testing.T) {
		b := New(1000)
		before := b.remaining
		s, err := AllocSlice[uint16](b, 100)
		if err != nil {
			t.Fatal(err)
		}
		if len(s) != 100 {
			t.Errorf("len = %d, want 100", len(s))
		}
		if got, want := before-b.remaining, int64(100*2+perAllocOverhead); got != want {
			t.Errorf("consumed = %d, want %d", got, want)
		}
	})

	t.Run("pointer", func(t *testing.T) {
		b := New(1000)
		before := b.remaining
		s, err := AllocSlice[*int](b, 4)
		if err != nil {
			t.Fatal(err)
		}
		if len(s) != 4 {
			t.Errorf("len = %d, want 4", len(s))
		}
		// 8 bytes per pointer on 64-bit
		if got, want := before-b.remaining, int64(4*8+perAllocOverhead); got != want {
			t.Errorf("consumed = %d, want %d", got, want)
		}
	})

	t.Run("exhaustion", func(t *testing.T) {
		b := New(10)
		s, err := AllocSlice[uint16](b, 1000)
		if !errors.Is(err, ErrExceeded) {
			t.Fatalf("err = %v, want ErrExceeded", err)
		}
		if s != nil {
			t.Errorf("got slice %v, want nil", s)
		}
		if b.remaining != 10 {
			t.Errorf("remaining = %d, want 10", b.remaining)
		}
	})

	t.Run("nil receiver", func(t *testing.T) {
		s, err := AllocSlice[uint16](nil, 5)
		if err != nil {
			t.Fatal(err)
		}
		if len(s) != 5 {
			t.Errorf("len = %d, want 5", len(s))
		}
	})

	t.Run("negative n", func(t *testing.T) {
		b := New(1000)
		if _, err := AllocSlice[uint16](b, -1); !errors.Is(err, ErrExceeded) {
			t.Fatalf("err = %v, want ErrExceeded", err)
		}
	})

	t.Run("multiplication overflow", func(t *testing.T) {
		// pick an n where n * sizeof(uint16) overflows
		s, err := AllocSlice[uint16](New(1<<40), int(^uint(0)>>1))
		if !errors.Is(err, ErrExceeded) {
			t.Fatalf("err = %v, want ErrExceeded", err)
		}
		if s != nil {
			t.Errorf("got slice %v, want nil", s)
		}
	})
}
