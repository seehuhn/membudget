# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v0.8.0] (2026-11-10)

This version has no changes.  It was tagged together with the other
Quire modules.

## [v0.7.6] (2026-11-10)

### Changed
- `AllocSlice` is now a generic method of `Budget`.  Calls change from
  `AllocSlice[T](b, n)` to `b.AllocSlice[T](n)`.  The package-level
  function is removed.
- The module requires Go 1.27.
- The module is now licensed under the BSD 3-Clause License.  The file
  `LICENSE` replaces `COPYING`.

## [v0.7.5] (2026-10-10)

This version has no changes.  It was tagged together with the other
Quire modules.

## [v0.7.4] (2026-06-25)

### Added
- `Budget.ChargeN` charges for `n` elements of a runtime-computed size,
  with overflow checking.
- `ErrInvalid`, returned when a requested size is not a sensible
  allocation size (negative count or too large to represent).

## [0.7.3] (2026-05-19)

Initial tagged release.

Provides a cumulative memory budget for parsing untrusted input,
extracted from seehuhn.de/go/sfnt and shared across the Quire modules
that need to bound peak allocations on adversarial files.

### Added
- `Budget` with `New`, `Charge`, and `Available`; safe for concurrent use.
- `AllocSlice[T]` generic helper that charges and allocates in one step,
  with overflow-safe size computation.
- `ErrExceeded` sentinel returned when a charge would push the budget
  below zero.
- Per-allocation surcharge to prevent amplification via many tiny
  allocations.
- `MapEntryOverhead` constant for callers charging map insertions.
